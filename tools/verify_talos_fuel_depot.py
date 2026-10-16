#!/usr/bin/env python3
"""在Docker中以原始68000直譯器驗證購油，入口為戰鬥研究及Issue #8。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def compare_formal(native, remake):
    """只比較真正執行的欄位；未支援輸入另外阻擋，不以零值當通過。"""
    report = {"FuelCases": 216, "AuxCases": 576, "FuelExecuted": 0,
              "AuxExecuted": 0, "FuelDifferences": 0, "AuxDifferences": 0,
              "UnsupportedFuelCases": 0, "UnsupportedAuxCases": 0,
              "Unsupported": {}, "Samples": []}
    for label, expected_count in (("Fuel", 216), ("Aux", 576)):
        original = native[label + "Native"]
        actual = remake[label]
        if len(original) != expected_count or len(actual) != expected_count:
            raise ValueError("正式／原版 caller 案數不同")
        for index, (expected, observed) in enumerate(zip(original, actual)):
            if observed["Index"] != index:
                raise ValueError("正式 caller 序號不同")
            if observed["Unsupported"]:
                report["Unsupported" + label + "Cases"] += 1
                for reason in observed["Unsupported"]:
                    report["Unsupported"][reason] = report["Unsupported"].get(reason, 0) + 1
            if not observed["Executed"]:
                if not observed["Unsupported"]:
                    raise ValueError("未執行 caller 卻未標示缺口")
                continue
            report[label + "Executed"] += 1
            fields = {"Bank": expected["ResultBank"], "Cash": expected["ResultCash"]}
            if label == "Fuel":
                fields["Fuel"] = expected["ResultFuel"]
            else:
                fields["Holds"] = [
                    {"Kind": word & 31, "Quality": (word >> 5) & 7, "Price": price, "SourceCode": word >> 8}
                    for word, price in zip(expected["Words"], expected["PriceWords"])]
            differences = {key: {"Original": value, "Remake": observed[key]}
                           for key, value in fields.items() if observed[key] != value}
            if differences:
                report[label + "Differences"] += 1
                if len(report["Samples"]) < 12:
                    report["Samples"].append({"Kind": label, "Index": index,
                                              "Input": expected, "Differences": differences})
    report["Scope"] = remake["Scope"]
    report["status"] = "PASS" if (report["FuelExecuted"] == 216
                                    and report["AuxExecuted"] == 576
                                    and not report["FuelDifferences"]
                                    and not report["AuxDifferences"]
                                    and not report["UnsupportedFuelCases"]
                                    and not report["UnsupportedAuxCases"]) else "FAIL"
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--talos", type=Path, required=True)
    parser.add_argument("--disk", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--require-formal", action="store_true",
                        help="同時執行正式 reducer；數值差異或未支援輸入均拒絕通過")
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只能在Docker執行")
    repo, talos, output = args.repo.resolve(), args.talos.resolve(), args.output.resolve()
    if output.exists():
        parser.error("輸出目錄已存在，保留既有收據")
    if not args.disk.is_file() or not (talos / "workplace/sundog/SYSTEM.INTERP").is_file():
        parser.error("磁碟或直譯器來源不存在")
    if output.parent.stat().st_uid != os.getuid():
        parser.error("輸出父目錄擁有者不符")
    output.mkdir(parents=True)
    names = ["aim", "cadence", "course", "init", "parent", "display", "levels", "movement", "fuel"]
    sources = [repo / f"tools/talos_combat_{n}_test.go.txt" for n in names]
    sources.append(repo / "tools/talos_fuel_depot_test.go.txt")
    mapping = {str(talos / f"internal/m68k/sundog_combat_{n}_test.go"): str(p)
               for n, p in zip(names, sources)}
    mapping[str(talos / "internal/m68k/sundog_fuel_depot_test.go")] = str(sources[-1])
    sources.append(repo / "tools/talos_fuel_entry_test.go.txt")
    mapping[str(talos / "internal/m68k/sundog_fuel_entry_test.go")] = str(sources[-1])
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": mapping}, indent=2) + "\n")
    env = os.environ.copy()
    env.update(SUNDOG_PARITY_DISK=str(args.disk.resolve()), SUNDOG_PARITY_OUTPUT=str(output))
    interp = talos / "workplace/sundog/SYSTEM.INTERP"
    if not interp.is_file():
        parser.error("找不到原始SYSTEM.INTERP")
    env.update(TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    command = ["go", "test", "-overlay", str(overlay), "./internal/m68k",
               "-run", "^TestSunDogFuel(Depot(Matrix|Chain)?|Entry|Controls|Energy)$", "-count=1", "-v"]
    with (output / "talos.log").open("w") as log:
        status = subprocess.run(command, cwd=talos, env=env, stdout=log, stderr=subprocess.STDOUT).returncode
    sources.append(Path(__file__).resolve())
    report = {"status": "PASS" if status == 0 else "FAIL", "issue": "https://github.com/wicanr2/sundog-remake-zh-hant/issues/8",
              "scope": "原生購油研究，非完整正式燃料鏈或正常玩家路徑",
              "command": command, "go": subprocess.check_output(["go", "version"], text=True).strip(),
              "source_sha256": {str(p.relative_to(repo)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sources},
              "disk_sha256": hashlib.sha256(args.disk.read_bytes()).hexdigest()}
    native = output / "native-fuel-depot.json"
    if native.is_file():
        data = json.loads(native.read_text())
        report["depot"] = {k: v for k, v in data.items() if not k.endswith("Native")}
        report["native_sha256"] = hashlib.sha256(native.read_bytes()).hexdigest()
    if not native.is_file() or "--- PASS: TestSunDogFuelDepot " not in (output / "talos.log").read_text():
        report["status"] = "FAIL"
        status = status or 1
    report["interp_sha256"] = hashlib.sha256(interp.read_bytes()).hexdigest()
    report["talos_head"] = subprocess.check_output(["git", "-C", str(talos), "rev-parse", "HEAD"], text=True).strip()
    matrix = output / "native-fuel-depot-matrix.json"
    if matrix.is_file():
        data = json.loads(matrix.read_text())
        report["matrix"] = {k: v for k, v in data.items() if not k.endswith("Native")}
        report["matrix_sha256"] = hashlib.sha256(matrix.read_bytes()).hexdigest()
    if not matrix.is_file() or "--- PASS: TestSunDogFuelDepotMatrix " not in (output / "talos.log").read_text():
        report["status"] = "FAIL"
        status = status or 1
    expected = {"FactorCases": 64, "PriceCases": 336, "WriteCases": 238,
                "AuxPriceCases": 128, "AuxWriteCases": 72, "PaymentCases": 1323}
    if report.get("depot", {}).get("Cases") != 6 or report.get("depot", {}).get("Differences") != 0:
        status = status or 1
    for key, count in expected.items():
        if report.get("matrix", {}).get(key) != count or report.get("matrix", {}).get(key.replace("Cases", "Differences")) != 0:
            status = status or 1
    report["status"] = "PASS" if status == 0 else "FAIL"
    chain = output / "native-fuel-depot-chain.json"
    if chain.is_file():
        data = json.loads(chain.read_text())
        report["chain"] = {k: v for k, v in data.items() if not k.endswith("Native")}
        report["chain_sha256"] = hashlib.sha256(chain.read_bytes()).hexdigest()
    if (not chain.is_file() or "--- PASS: TestSunDogFuelDepotChain " not in (output / "talos.log").read_text()
            or report.get("chain", {}).get("FuelCases") != 216
            or report.get("chain", {}).get("AuxCases") != 576
            or report.get("chain", {}).get("FuelDifferences") != 0
            or report.get("chain", {}).get("AuxDifferences") != 0):
        status = status or 1
    entry = output / "native-fuel-entry.json"
    expected_groups = {"boot-and-new-game": 16, "same-state-return-chain": 32,
                       "airlock-exit": 24, "pod-exit": 24, "board-success": 8,
                       "ship-entry": 8, "ground-enter-vehicle": 8, "movement-pattern": 8}
    if entry.is_file():
        data = json.loads(entry.read_text())
        report["entry"] = {k: v for k, v in data.items() if k not in ("Native", "GateNative")}
        report["entry_sha256"] = hashlib.sha256(entry.read_bytes()).hexdigest()
    if (not entry.is_file() or "--- PASS: TestSunDogFuelEntry " not in (output / "talos.log").read_text()
            or report.get("entry", {}).get("Cases") != 128
            or report.get("entry", {}).get("Differences") != 0
            or report.get("entry", {}).get("GateCases") != 32
            or report.get("entry", {}).get("GateDifferences") != 0
            or report.get("entry", {}).get("Groups") != expected_groups):
        status = status or 1
    controls = output / "native-fuel-controls.json"
    if controls.is_file():
        data = json.loads(controls.read_text())
        report["controls"] = {k: v for k, v in data.items() if k != "Native"}
        report["controls_sha256"] = hashlib.sha256(controls.read_bytes()).hexdigest()
    if (not controls.is_file() or "--- PASS: TestSunDogFuelControls " not in (output / "talos.log").read_text()
            or report.get("controls", {}).get("Cases") != 396
            or report.get("controls", {}).get("Differences") != 0):
        status = status or 1
    energy = output / "native-fuel-energy.json"
    if energy.is_file():
        data = json.loads(energy.read_text())
        report["energy"] = {k: v for k, v in data.items() if k not in ("Native", "ToggleNative")}
        report["energy_sha256"] = hashlib.sha256(energy.read_bytes()).hexdigest()
    if (not energy.is_file() or "--- PASS: TestSunDogFuelEnergy " not in (output / "talos.log").read_text()
            or report.get("energy", {}).get("Cases") != 2194
            or report.get("energy", {}).get("Differences") != 0
            or report.get("energy", {}).get("ToggleCases") != 16
            or report.get("energy", {}).get("ToggleDifferences") != 0
            or report.get("energy", {}).get("Groups") != {
                "speed": 1458, "tractor-cloak": 288, "distress": 192, "jettison": 64,
                "speed-aux": 144, "tractor-cloak-aux": 48}):
        status = status or 1
    if args.require_formal and status == 0:
        formal_command = ["go", "test", "./internal/game", "-run",
                          "^TestExportTalosFuel(Depot|Energy)$", "-count=1", "-v"]
        with (output / "formal.log").open("w") as log:
            formal_status = subprocess.run(formal_command, cwd=repo, env=env,
                                          stdout=log, stderr=subprocess.STDOUT).returncode
        report["formal_command"] = formal_command
        formal = output / "remake-fuel-depot.json"
        if (formal_status != 0 or not formal.is_file()
                or "--- PASS: TestExportTalosFuelDepot " not in (output / "formal.log").read_text()):
            status = status or 1
            report["formal_error"] = "正式匯出失敗或缺測"
        else:
            report["formal"] = compare_formal(json.loads(chain.read_text()), json.loads(formal.read_text()))
            report["formal_sha256"] = hashlib.sha256(formal.read_bytes()).hexdigest()
            if report["formal"]["status"] != "PASS":
                status = status or 1

        formal_energy = output / "remake-fuel-energy.json"
        formal_log = (output / "formal.log").read_text()
        if (formal_status != 0 or not formal_energy.is_file()
                or "--- PASS: TestExportTalosFuelEnergy " not in formal_log):
            status = status or 1
            report["formal_energy_error"] = "正式耗能匯出失敗或缺測"
        else:
            data = json.loads(formal_energy.read_text())
            original = json.loads(energy.read_text())
            same_rows = data.get("Native") == original.get("Native")
            report["formal_energy"] = {k: v for k, v in data.items() if k != "Native"}
            report["formal_energy"]["SameRows"] = same_rows
            report["formal_energy_sha256"] = hashlib.sha256(formal_energy.read_bytes()).hexdigest()
            if (data.get("Cases") != 2194 or data.get("Differences") != 0
                    or data.get("Groups") != report["energy"]["Groups"] or not same_rows):
                status = status or 1
        for path in subprocess.check_output(
                ["git", "ls-files", "internal/game/*.go", "internal/world/*.go", "internal/storage/*.go"],
                cwd=repo, text=True).splitlines() + ["internal/game/talos_fuel_depot_fixture_test.go",
                    "internal/game/talos_fuel_energy_fixture_test.go", "internal/game/fuel_purchase.go",
                    "internal/game/fuel_state.go", "internal/game/ship_energy.go", "internal/storage/current_state.go"]:
            report["source_sha256"][path] = hashlib.sha256((repo / path).read_bytes()).hexdigest()
    report["status"] = "PASS" if status == 0 else "FAIL"
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print((output / "talos.log").read_text())
    print(json.dumps({"status": report["status"], "depot": report.get("depot", {}), "matrix": report.get("matrix", {}), "chain": report.get("chain", {}), "entry": report.get("entry", {}), "controls": report.get("controls", {}), "energy": report.get("energy", {}), "formal": report.get("formal", {}), "formal_energy": report.get("formal_energy", {}), "formal_error": report.get("formal_error"), "formal_energy_error": report.get("formal_energy_error")}, ensure_ascii=False))
    raise SystemExit(status)


if __name__ == "__main__":
    main()
