#!/usr/bin/env python3
"""由物品／貨物 evidence 與譯名表產生玩家可見的 catalog 條目。

與地名相同的做法：譯名集中在 ``docs/i18n/atari-st-item-names-zh-Hant.json``，catalog 的
``ui.item.*``／``ui.cargo.*`` 由本工具產生，缺譯或譯名表多出來源沒有的名稱時 fail-closed。
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

MANAGED_PREFIXES = ("ui.item.", "ui.cargo.")


def slug(name: str) -> str:
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


def build_entries(evidence: dict, names: dict) -> tuple[list[dict], list[str]]:
    errors: list[str] = []
    entries: list[dict] = []
    for kind, prefix in (("items", "ui.item."), ("cargo", "ui.cargo.")):
        table = {record["name"]: record for record in evidence[kind]["records"]}
        translations = names.get(kind, {})
        extra = sorted(set(translations) - set(table))
        if extra:
            errors.append(f"{kind} translations name sources absent from the evidence: {extra}")
        for name, record in table.items():
            value = translations.get(name)
            if not value:
                errors.append(f"{kind} name {name!r} has no zh-Hant translation")
                continue
            entries.append(
                {
                    "key": prefix + slug(name),
                    "source": name,
                    "translation": value,
                    "translationStatus": "draft",
                }
            )
    return entries, errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, default=Path("docs/atari-st/item-data.json"))
    parser.add_argument("--names", type=Path, default=Path("docs/i18n/atari-st-item-names-zh-Hant.json"))
    parser.add_argument("--catalog", type=Path, default=Path("docs/i18n/sundog-ui-zh-Hant.json"))
    parser.add_argument("--check", action="store_true", help="只比對，不寫回 catalog")
    args = parser.parse_args()

    evidence = json.loads(args.evidence.read_text(encoding="utf-8"))
    names = json.loads(args.names.read_text(encoding="utf-8"))
    entries, errors = build_entries(evidence, names)

    catalog = json.loads(args.catalog.read_text(encoding="utf-8"))
    kept = [entry for entry in catalog["entries"] if not entry["key"].startswith(MANAGED_PREFIXES)]
    existing = {
        entry["key"]: entry
        for entry in catalog["entries"]
        if entry["key"].startswith(MANAGED_PREFIXES)
    }
    orphan = sorted(set(existing) - {entry["key"] for entry in entries})
    if orphan:
        errors.append(f"catalog carries managed keys with no source: {orphan}")
    changed = sum(
        1
        for entry in entries
        if entry["key"] not in existing
        or existing[entry["key"]].get("translation") != entry["translation"]
    )

    if not errors and not args.check:
        catalog["entries"] = kept + entries
        args.catalog.write_text(
            json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )

    print(json.dumps({
        "status": "pass" if not errors else "fail",
        "mode": "check" if args.check else "write",
        "generatedEntryCount": len(entries),
        "changedKeyCount": changed,
        "errors": errors,
    }, ensure_ascii=False, indent=2, sort_keys=True))
    if errors:
        return 1
    return 1 if args.check and changed else 0


if __name__ == "__main__":
    sys.exit(main())
