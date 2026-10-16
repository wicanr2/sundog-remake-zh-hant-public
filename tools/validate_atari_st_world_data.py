#!/usr/bin/env python3
"""驗證 Atari ST 世界資料 evidence 檔的自洽性與內嵌副本一致性。

驗證項目不讀原始磁碟，只看已提交的 JSON：

- 三張表的 record 數、stride、raw offset 是否首尾相接且連續。
- 每筆的 name 長度、tail hex 長度與 recordSha256 欄位是否齊全。
- planet→city 與 system→planet 的區間是否完整覆蓋且不重疊。
- ``internal/source/atarist/assets/world-data.json`` 是否與 evidence 檔逐 byte 相同。
  兩份檔案必須一致，因為 go:embed 無法引用套件目錄以外的路徑。
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

NAME_FIELD_SIZE = 10
EXPECTED = {
    "systems": {"recordSize": 20, "recordCount": 12},
    "planets": {"recordSize": 16, "recordCount": 18},
    "cities": {"recordSize": 12, "recordCount": 60},
}
TABLE_ORDER = ("systems", "planets", "cities")


def validate_document(document: dict) -> list[str]:
    errors: list[str] = []
    if document.get("schema") != 1:
        errors.append(f"unexpected schema: {document.get('schema')!r}")

    tables = document.get("tables")
    if not isinstance(tables, dict):
        return errors + ["tables is missing"]

    cursor: int | None = None
    for kind in TABLE_ORDER:
        table = tables.get(kind)
        if not isinstance(table, dict):
            errors.append(f"{kind} table is missing")
            continue
        expected = EXPECTED[kind]
        if table.get("recordSize") != expected["recordSize"]:
            errors.append(f"{kind} recordSize is {table.get('recordSize')}, want {expected['recordSize']}")
        if table.get("recordCount") != expected["recordCount"]:
            errors.append(f"{kind} recordCount is {table.get('recordCount')}, want {expected['recordCount']}")
        records = table.get("records")
        if not isinstance(records, list) or len(records) != expected["recordCount"]:
            errors.append(f"{kind} carries {len(records) if isinstance(records, list) else 'no'} records")
            continue

        start = int(str(table.get("rawOffset")), 0)
        if cursor is not None and start != cursor:
            errors.append(f"{kind} starts at 0x{start:05x}, want 0x{cursor:05x}")
        cursor = start + expected["recordSize"] * expected["recordCount"]

        tail_chars = (expected["recordSize"] - NAME_FIELD_SIZE) * 2
        for index, record in enumerate(records):
            if record.get("index") != index:
                errors.append(f"{kind} record {index} declares index {record.get('index')}")
            offset = int(str(record.get("rawOffset")), 0)
            want = start + index * expected["recordSize"]
            if offset != want:
                errors.append(f"{kind} record {index} is at 0x{offset:05x}, want 0x{want:05x}")
            name = record.get("name")
            if not isinstance(name, str) or not name or len(name) > NAME_FIELD_SIZE - 1:
                errors.append(f"{kind} record {index} has an invalid name {name!r}")
            tail = record.get("tailHex")
            if not isinstance(tail, str) or len(tail) != tail_chars:
                errors.append(f"{kind} record {index} tail is {tail!r}, want {tail_chars} hex chars")
            if not isinstance(record.get("recordSha256"), str):
                errors.append(f"{kind} record {index} has no recordSha256")

    region = document.get("region")
    if isinstance(region, dict) and cursor is not None:
        used = region.get("usedBytes")
        start = int(str(region.get("rawStart")), 0)
        if used != cursor - start:
            errors.append(f"region usedBytes is {used}, want {cursor - start}")
        if region.get("trailerIsZero") is not True:
            errors.append("region trailer is not recorded as zero-filled")

    grouping = document.get("grouping")
    if not isinstance(grouping, dict):
        return errors + ["grouping is missing"]

    planet_names = [record["name"] for record in tables["planets"]["records"]]
    system_names = [record["name"] for record in tables["systems"]["records"]]

    cursor = 0
    for index, run in enumerate(grouping.get("planetCityRuns", [])):
        if index >= len(planet_names) or run.get("planet") != planet_names[index]:
            errors.append(f"planet run {index} names {run.get('planet')!r}")
        if run.get("start") != cursor:
            errors.append(f"planet run {index} starts at {run.get('start')}, want {cursor}")
        cursor += int(run.get("count", 0))
    if cursor != grouping.get("contiguousCityCount"):
        errors.append(f"planet runs cover {cursor} cities, declared {grouping.get('contiguousCityCount')}")

    cursor = 0
    for index, run in enumerate(grouping.get("systemPlanetRuns", [])):
        if index >= len(system_names) or run.get("system") != system_names[index]:
            errors.append(f"system run {index} names {run.get('system')!r}")
        if run.get("start") != cursor:
            errors.append(f"system run {index} starts at {run.get('start')}, want {cursor}")
        cursor += int(run.get("count", 0))
    if cursor != len(planet_names):
        errors.append(f"system runs cover {cursor} planets, want {len(planet_names)}")

    trailing = grouping.get("trailingCities", [])
    if len(trailing) != grouping.get("trailingCityCount"):
        errors.append("trailingCities length does not match trailingCityCount")
    for entry in trailing:
        if entry.get("planet") not in planet_names:
            errors.append(f"trailing city {entry.get('city')!r} names unknown planet {entry.get('planet')!r}")

    total = (
        int(grouping.get("contiguousCityCount", 0))
        + int(grouping.get("fillerCityCount", 0))
        + int(grouping.get("trailingCityCount", 0))
    )
    if total != EXPECTED["cities"]["recordCount"]:
        errors.append(f"grouping accounts for {total} cities, want {EXPECTED['cities']['recordCount']}")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "evidence",
        type=Path,
        nargs="?",
        default=Path("docs/atari-st/world-data.json"),
    )
    parser.add_argument(
        "--embedded",
        type=Path,
        default=Path("internal/source/atarist/assets/world-data.json"),
    )
    args = parser.parse_args()

    evidence_bytes = args.evidence.read_bytes()
    errors = validate_document(json.loads(evidence_bytes.decode("utf-8")))

    if not args.embedded.exists():
        errors.append(f"embedded copy is missing: {args.embedded}")
    elif args.embedded.read_bytes() != evidence_bytes:
        errors.append(f"embedded copy differs from {args.evidence}")

    result = {
        "status": "pass" if not errors else "fail",
        "evidence": str(args.evidence),
        "embedded": str(args.embedded),
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
