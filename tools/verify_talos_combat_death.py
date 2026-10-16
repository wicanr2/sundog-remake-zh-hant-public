#!/usr/bin/env python3
"""Docker內致命攻擊當拍父鏈對拍；規格入口docs/atari-st/ship-rules.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

from verify_talos_ground_collision import run_logged


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


FIELDS = ("Phase", "Fuel", "Weapon", "Shield", "Setting", "Hull", "Cooldown",
          "Range", "Bearing", "Pitch", "BearingDrift", "PitchDrift", "Counter",
          "Words", "Systems", "Timer", "Ready", "Closing", "Aux8", "Aux9", "CanPrepareFire")


def compare(native, formal):
    key = lambda item: json.dumps(item, sort_keys=True)
    wanted = {key(dict(Enemy=e, Mode=m, Phase=p, Hull=h, Seed=s, Layout=layout))
              for e in range(7) for m in range(8) for p in range(3)
              for h in (1, 2, 3, 4, 16) for s in (0, 1, 17, 41) for layout in (0, 1)}
    coverage = all(len(rows) == len(wanted) and {key(r.get("Input")) for r in rows} == wanted
                   for rows in (native, formal))
    index = {key(r["Input"]): r for r in native}
    differences = {field: 0 for field in FIELDS}
    differences.update(Terminal=0, InitialSystems=0)
    examples = []
    deaths = 0
    for row in formal:
        original = index.get(key(row["Input"]))
        if original is None:
            coverage = False
            continue
        a, b = original["Step"], row["Step"]
        flags = a.get("Flags", [])
        coverage &= len(flags) == 5
        if len(flags) != 5:
            continue
        a = dict(a, CanPrepareFire=bool(flags[44 // 16] & (1 << (44 % 16))))
        coverage &= a.get("Phase") == (row["Input"]["Phase"] + 1) % 3
        coverage &= not bool(flags[0] & (1 << 12))
        draws = original.get("Draws", [])
        coverage &= bool(draws) and all(d["Counter"] == row["Input"]["Seed"] + n + 1
                                      and d["Low"] <= d["Value"] <= d["High"]
                                      for n, d in enumerate(draws))
        coverage &= bool(draws) and a.get("Counter") == row["Input"]["Seed"] + len(draws)
        calls = [c for c in original.get("Calls", []) if c[0] == 0x31e00 and c[1] in (0x1295, 0x1298, 0x12a8)]
        coverage &= calls == [[0x31e00, at, proc] for at, proc in ((0x1295, 0x1f), (0x1298, 0x23), (0x12a8, 0x21))]
        coverage &= set(a) == set(FIELDS) | {"Active", "Flags", "RandomWords"}
        coverage &= set(b) == set(FIELDS) | {"Active"}
        coverage &= a.get("Active") is True
        for systems in (a.get("Systems", []), b.get("Systems", []), row.get("InitialSystems", []), original.get("InitialSystems", [])):
            coverage &= len(systems) == 6 and all(len(s) == 18 for s in systems)
        expected_status = [16, 16, 8, 16, 8, 16] if row["Input"]["Layout"] == 0 else [0, 16, 0, 0, 0, 0]
        for systems in (row.get("InitialSystems", []), original.get("InitialSystems", [])):
            coverage &= len(systems) == 6 and all(len(s) == 18 for s in systems)
            if len(systems) == 6 and all(len(s) == 18 for s in systems):
                coverage &= [s[16] for s in systems] == expected_status
        if row.get("InitialSystems") != original.get("InitialSystems"):
            differences["InitialSystems"] += 1
        dead = a.get("Hull") == 0
        deaths += int(dead)
        # The native local encounter-active bit remains true on its lethal
        # return. Keep it raw, and compare the explicit terminal projection
        # separately rather than relabeling it as the remake active field.
        markers = (original.get("Global3"), original.get("DisplayMode"), original.get("Cause"))
        terminal_ok = (markers == (1, 12, 2) and row.get("Screen") == "outcome"
                       and row.get("Outcome") == "ui.outcome.failure" and not b.get("Active")
                       and not row.get("SleepActive")) if dead else (
                       markers == (0, 7, 0) and row.get("Screen") == "tactical"
                       and b.get("Active") == a.get("Active") and row.get("Outcome") == ""
                       and not row.get("SleepActive"))
        if not terminal_ok:
            differences["Terminal"] += 1
        for field in FIELDS:
            if field not in a or field not in b:
                coverage = False
            elif a[field] != b[field]:
                differences[field] += 1
                if len(examples) < 12:
                    examples.append(dict(input=row["Input"], field=field, native=a[field], formal=b[field]))
    coverage &= 0 < deaths < len(wanted)
    return dict(status="PASS-scoped" if coverage and not any(differences.values()) else "FAIL",
                cases=len(formal), completeUniqueInputs=bool(coverage), deaths=deaths,
                survivors=len(formal)-deaths, differences=differences, examples=examples,
                comparedPhysicalFields=len(FIELDS),
                terminalProjection="native Global3/mode/cause=1/12/2 -> formal failure; native local Active retained raw",
                originalFullDeathGUI="unverified")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("disk", "talos", "output"):
        parser.add_argument("--"+name, type=Path, required=True)
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只能在Docker執行")
    root = Path(__file__).resolve().parents[1]
    disk, talos, out = args.disk.resolve(strict=True), args.talos.resolve(strict=True), args.output.resolve()
    interp = talos/"workplace/sundog/SYSTEM.INTERP"
    if sha(disk) != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6":
        parser.error("原盤身分不同")
    if sha(interp) != "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":
        parser.error("原始直譯器身分不同")
    if out.exists() or out.parent.stat().st_uid != os.getuid():
        parser.error("須使用擁有者相符的新輸出")
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if version.split()[2] != "go1.25.12":
        parser.error("須沿用Go1.25.12")
    out.mkdir(mode=0o700)
    shutil.copyfile(Path(__file__), out/"tool-at-start.py")
    names = ("combat_aim", "combat_cadence", "combat_course", "combat_init", "combat_parent",
             "combat_display", "combat_levels", "combat_movement", "combat_fuel", "fuel_depot",
             "fuel_entry", "walker_slices", "banville", "banville_price", "banville_site",
             "trade_classes", "food", "flight_clock", "combat_death")
    sources = [root/f"tools/talos_{name}_test.go.txt" for name in names]
    overlay = out/"overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(talos/f"internal/m68k/sundog_{name}_test.go"): str(p)
                                               for name, p in zip(names, sources)}}))
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(out),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    commands = [
        ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
         "-run", "^TestSunDogCombatLethalParent$", "-count=1", "-v"],
        ["go", "test", "./internal/game", "-run", "^TestExportTalosCombatLethalParent$", "-count=1", "-v"]]
    for cmd, cwd, log in zip(commands, (talos, root), ("native.log", "formal.log")):
        run_logged(cmd, out/log, cwd=cwd, env=env, timeout=320)
    paths = [out/name for name in ("native-combat-lethal.json", "remake-combat-lethal.json")]
    report = compare(*(json.loads(p.read_text()) for p in paths))
    inputs = sources + [Path(__file__).resolve(), root/"internal/game/talos_combat_death_fixture_test.go",
                        root/"internal/game/combat.go", root/"internal/game/enemy_course.go"]
    report.update(go=version, diskSHA256=sha(disk), interpreterSHA256=sha(interp),
                  talosHead=subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
                  sourceSHA256={str(p.relative_to(root)): sha(p) for p in inputs},
                  outputSHA256={p.name: sha(p) for p in paths}, commands=commands,
                  randomness="Predeclared counter0/1/17/41; original instructions choose bounds/order, same bounded sequence as combatRoll. Not original RNG equality.",
                  locations={"rawBase": "0x31e00", "parent": "0x1278..0x12c1", "attack": "0x0d23..0x0f65",
                             "hullZero": "0x0e79..0x0e8e", "overflowRepeat": "0x0f1a..0x0f24"},
                  scope="One prepared-shot parent from each initial state; physical state, source lethal markers and typed terminal projection. No full original death GUI/boot.")
    (out/"receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n")
    print(f"{report['status']}: {report['cases']} prepared-shot cases; {report['deaths']} lethal, differences={report['differences']}")
    if report["status"] != "PASS-scoped":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
