#!/usr/bin/env python3
"""由世界資料表與譯名表產生玩家可見的地名 catalog 條目。

輸入是 ``docs/atari-st/world-data.json``（來源名稱與分組）與
``docs/i18n/atari-st-world-names-zh-Hant.json``（譯名）。輸出直接寫回繁中 UI
overlay 的 ``ui.system.*``／``ui.planet.*``／``ui.city.*``／``ui.destination.*``。

譯名不手改 catalog：手改會讓 88 個地名散落在一份 300 多筆的檔案裡，之後無從比對
來源是否漂移。這支工具讓譯名表成為唯一可編輯處，並在來源名稱缺譯或譯名表多出
來源沒有的名稱時 fail-closed。

顯示格式由譯名表的 ``displayFormat`` 決定；使用者於 2026-08-16 定為「中文(原文)」。
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

FILLER_CITY_NAME = "dummy"
MANAGED_PREFIXES = ("ui.system.", "ui.planet.", "ui.city.", "ui.destination.")


def slug(name: str) -> str:
    """與 internal/source/atarist 的 slug 規則一致的穩定識別字。"""
    out: list[str] = []
    dash = True
    for char in name.lower():
        if char.isascii() and (char.isalpha() or char.isdigit()):
            out.append(char)
            dash = False
        elif not dash:
            out.append("-")
            dash = True
    return "".join(out).strip("-")


def render(display_format: str, zh: str, source: str) -> str:
    return display_format.format(zh=zh, source=source)


def build_entries(world: dict, names: dict) -> tuple[list[dict], list[str]]:
    errors: list[str] = []
    display_format = names.get("displayFormat", "{zh}({source})")

    tables = world["tables"]
    systems = [record["name"] for record in tables["systems"]["records"]]
    planets = [record["name"] for record in tables["planets"]["records"]]
    cities = [record["name"] for record in tables["cities"]["records"]]
    grouping = world["grouping"]

    system_of_planet: dict[str, str] = {}
    for run in grouping["systemPlanetRuns"]:
        for index in range(run["start"], run["start"] + run["count"]):
            system_of_planet[planets[index]] = run["system"]
    planet_of_city: dict[str, str] = {}
    for run in grouping["planetCityRuns"]:
        for index in range(run["start"], run["start"] + run["count"]):
            planet_of_city[cities[index]] = run["planet"]
    for entry in grouping["trailingCities"]:
        planet_of_city[entry["city"]] = entry["planet"]

    def lookup(kind: str, source: str) -> str | None:
        table = names.get(kind, {})
        value = table.get(source)
        if not value:
            errors.append(f"{kind} name {source!r} has no zh-Hant translation")
            return None
        return value

    # 譯名表不得多出來源表沒有的名稱：那通常表示來源已改，而譯名還停在舊版。
    for kind, sources in (
        ("systems", systems),
        ("planets", planets),
        ("cities", [name for name in cities if name != FILLER_CITY_NAME]),
    ):
        extra = sorted(set(names.get(kind, {})) - set(sources))
        if extra:
            errors.append(f"{kind} translations name sources absent from the world data: {extra}")

    entries: list[dict] = []
    for source in systems:
        zh = lookup("systems", source)
        if zh:
            entries.append(
                {
                    "key": f"ui.system.{slug(source)}",
                    "source": source,
                    "translation": render(display_format, zh, source),
                    "translationStatus": "draft",
                }
            )
    for source in planets:
        zh = lookup("planets", source)
        if zh:
            entries.append(
                {
                    "key": f"ui.planet.{slug(source)}",
                    "source": source,
                    "translation": render(display_format, zh, source),
                    "translationStatus": "draft",
                }
            )
    for source in cities:
        if source == FILLER_CITY_NAME:
            continue
        zh = lookup("cities", source)
        if not zh:
            continue
        city_label = render(display_format, zh, source)
        entries.append(
            {
                "key": f"ui.city.{slug(source)}",
                "source": source,
                "translation": city_label,
                "translationStatus": "draft",
            }
        )
        planet_source = planet_of_city.get(source)
        planet_zh = names.get("planets", {}).get(planet_source or "")
        if not planet_source or not planet_zh:
            errors.append(f"city {source!r} has no translated planet")
            continue
        entries.append(
            {
                "key": f"ui.destination.{slug(source)}",
                "source": f"{system_of_planet.get(planet_source, '')}/{planet_source}/{source}",
                "translation": f"{render(display_format, planet_zh, planet_source)}／{city_label}",
                "translationStatus": "draft",
            }
        )
    return entries, errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--world", type=Path, default=Path("docs/atari-st/world-data.json"))
    parser.add_argument("--names", type=Path, default=Path("docs/i18n/atari-st-world-names-zh-Hant.json"))
    parser.add_argument("--catalog", type=Path, default=Path("docs/i18n/sundog-ui-zh-Hant.json"))
    parser.add_argument("--check", action="store_true", help="只比對，不寫回 catalog")
    args = parser.parse_args()

    world = json.loads(args.world.read_text(encoding="utf-8"))
    names = json.loads(args.names.read_text(encoding="utf-8"))
    entries, errors = build_entries(world, names)

    catalog = json.loads(args.catalog.read_text(encoding="utf-8"))
    kept = [
        entry
        for entry in catalog["entries"]
        if not entry["key"].startswith(MANAGED_PREFIXES)
    ]
    existing = {
        entry["key"]: entry
        for entry in catalog["entries"]
        if entry["key"].startswith(MANAGED_PREFIXES)
    }

    drifted = [
        entry["key"]
        for entry in entries
        if entry["key"] in existing
        and existing[entry["key"]].get("translation") != entry["translation"]
    ]
    missing = [entry["key"] for entry in entries if entry["key"] not in existing]
    orphan = sorted(set(existing) - {entry["key"] for entry in entries})
    if orphan:
        errors.append(f"catalog carries managed keys with no source: {orphan}")

    if not errors and not args.check:
        catalog["entries"] = kept + entries
        args.catalog.write_text(
            json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )

    result = {
        "status": "pass" if not errors else "fail",
        "mode": "check" if args.check else "write",
        "generatedEntryCount": len(entries),
        "unmanagedEntryCount": len(kept),
        "changedKeyCount": len(drifted) + len(missing),
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    if errors:
        return 1
    if args.check and (drifted or missing):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
