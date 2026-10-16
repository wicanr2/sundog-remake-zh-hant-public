"""Docker 原版瞄準切片與正式規則比較；入口見 talos-combat-audit-20260907.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

from verify_talos_ground_collision import run_logged


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--disk", type=Path, required=True)
    parser.add_argument("--talos", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只能在 Docker 執行")
    repo = Path(__file__).resolve().parents[1]
    disk, talos, output = args.disk.resolve(), args.talos.resolve(), args.output.resolve()
    interp = talos / "workplace/sundog/SYSTEM.INTERP"
    if sha(disk) != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6":
        parser.error("磁碟身分不同")
    if sha(interp) != "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":
        parser.error("直譯器身分不同")
    if output.exists() and (not output.is_dir() or any(output.iterdir())):
        parser.error("輸出不可覆寫歷史收據")
    if output.parent.stat().st_uid != os.getuid():
        parser.error("輸出父目錄擁有者不符")
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if version.split()[2] != "go1.25.12":
        parser.error("須沿用 Go 1.25.12 固定工具鏈")
    output.mkdir(mode=0o700, exist_ok=True)
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    source = repo / "tools/talos_combat_aim_test.go.txt"
    run_logged(["go", "test", "./internal/game", "-run", "^TestExportTalosCombat(Aim|Slew|Damage|EnemyDamage|HullDamage|Overflow)$", "-count=1", "-v"],
               output / "remake.log", cwd=repo, env=env)
    with tempfile.TemporaryDirectory(prefix="sundog-aim-overlay-") as temp:
        overlay = Path(temp) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {
            str(talos / "internal/m68k/sundog_combat_aim_test.go"): str(source)}}), encoding="utf-8")
        status = run_logged(["go", "test", "-overlay", str(overlay), "./internal/m68k",
                             "-run", "^TestSunDogCombat(Aim|Slew|Damage|EnemyDamage|HullDamage|Overflow)$", "-count=1", "-v"],
                            output / "talos.log", cwd=talos, env=env, allow_failure=True)
    native = json.loads((output / "native-aim.json").read_text(encoding="utf-8"))
    if status == 0 and any("--- PASS: "+name+" " not in (output / "talos.log").read_text()
                           for name in ("TestSunDogCombatAim", "TestSunDogCombatSlew", "TestSunDogCombatDamage", "TestSunDogCombatEnemyDamage", "TestSunDogCombatHullDamage", "TestSunDogCombatOverflow")):
        raise RuntimeError("缺少必要原版測試，不能宣稱成功")
    sources = sorted(p for p in (repo / "internal").rglob("*") if p.is_file() and p.suffix in (".go", ".json"))
    sources.extend([source, Path(__file__).resolve(), repo / "go.mod", repo / "go.sum"])
    report = {"issue": "https://github.com/wicanr2/sundog-remake-zh-hant/issues/6",
              "scope": native["Scope"], "status": "PASS" if status == 0 else "FAIL",
              "cases": native["Cases"], "window_differences": native["WindowDifferences"],
              "hit_differences": native["HitDifferences"],
              "randomness": "瞄準／轉向分支無亂數；傷害兩側在執行前固定 Seed，以明示同 counter 測試序列供應原版 rand 呼叫，非原版 random() 對拍",
              "go_version": version, "disk_sha256": sha(disk), "interp_sha256": sha(interp),
              "source_sha256": {str(p.relative_to(repo)): sha(p) for p in sources},
              "talos_source_sha256": {str(p.relative_to(talos)): sha(p)
                  for p in sorted((talos / "internal/m68k").glob("*.go"))},
              "artifacts": {p.name: sha(p) for p in sorted(output.iterdir()) if p.is_file()}}
    slew = output / "native-slew.json"
    if slew.is_file():
        data = json.loads(slew.read_text(encoding="utf-8"))
        report.update(slew_cases=data["Cases"], slew_differences=data["Differences"], slew_scope=data["Scope"])
    damage = output / "native-damage.json"
    if damage.is_file():
        data = json.loads(damage.read_text(encoding="utf-8"))
        report.update(damage_cases=data["Cases"], damage_differences=data["Differences"],
                      damage_scope=data["Scope"], damage_randomness=data["Randomness"])
    enemy_damage = output / "native-enemy-damage.json"
    if enemy_damage.is_file():
        data = json.loads(enemy_damage.read_text(encoding="utf-8"))
        report.update(enemy_damage_cases=data["Cases"], enemy_damage_differences=data["Differences"],
                      enemy_damage_scope=data["Scope"], enemy_damage_randomness=data["Randomness"])
    hull_damage = output / "native-hull-damage.json"
    if hull_damage.is_file():
        data = json.loads(hull_damage.read_text(encoding="utf-8"))
        report.update(hull_damage_cases=data["Cases"], hull_damage_differences=data["Differences"],
                      hull_damage_scope=data["Scope"], hull_damage_randomness=data["Randomness"])
    overflow = output / "native-overflow.json"
    if overflow.is_file():
        data = json.loads(overflow.read_text(encoding="utf-8"))
        report.update(overflow_cases=data["Cases"], overflow_differences=data["Differences"],
                      overflow_scope=data["Scope"], overflow_randomness=data["Randomness"],
                      overflow_fields={key: data[key] for key in
                          ("HullDifferences", "ShieldDifferences", "SystemDifferences", "CounterDifferences",
                           "ChargeDifferences", "SettingDifferences")})
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
    print(f"原版瞄準切片 {report['status']}：{output / 'receipt.json'}")
    if status:
        raise RuntimeError("原版／重製版瞄準差異，見保存收據")


if __name__ == "__main__":
    main()
