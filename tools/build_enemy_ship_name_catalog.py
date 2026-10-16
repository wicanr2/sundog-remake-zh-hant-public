#!/usr/bin/env python3
"""由敵艦資料表與譯名表產生玩家可見的敵艦型號 catalog 條目。

與地名相同的做法：譯名集中在 ``docs/i18n/atari-st-enemy-ship-names-zh-Hant.json``，
catalog 的 ``ui.enemy.*`` 由本工具產生。缺譯、或譯名表多出來源沒有的型號時 fail-closed
——後者通常表示來源表已改而譯名停在舊版。

顯示格式由譯名表的 ``displayFormat`` 決定；敵艦型號是專有名詞，沿用地名的「中文(原文)」。
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

MANAGED_PREFIX = "ui.enemy."


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
    display_format = names.get("displayFormat", "{zh}({source})")
    translations = names.get("ships", {})
    sources = [ship["name"] for ship in evidence["ships"]]

    extra = sorted(set(translations) - set(sources))
    if extra:
        errors.append(f"translations name enemy ships absent from the evidence: {extra}")

    entries: list[dict] = []
    for name in sources:
        value = translations.get(name)
        if not value:
            errors.append(f"enemy ship {name!r} has no zh-Hant translation")
            continue
        entries.append(
            {
                "key": MANAGED_PREFIX + slug(name),
                "source": name,
                "translation": display_format.format(zh=value, source=name),
                "translationStatus": "draft",
            }
        )
    return entries, errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, default=Path("docs/atari-st/enemy-ships.json"))
    parser.add_argument(
        "--names", type=Path, default=Path("docs/i18n/atari-st-enemy-ship-names-zh-Hant.json")
    )
    parser.add_argument("--catalog", type=Path, default=Path("docs/i18n/sundog-ui-zh-Hant.json"))
    parser.add_argument("--check", action="store_true", help="只比對，不寫回 catalog")
    args = parser.parse_args()

    evidence = json.loads(args.evidence.read_text(encoding="utf-8"))
    names = json.loads(args.names.read_text(encoding="utf-8"))
    entries, errors = build_entries(evidence, names)

    catalog = json.loads(args.catalog.read_text(encoding="utf-8"))
    kept = [entry for entry in catalog["entries"] if not entry["key"].startswith(MANAGED_PREFIX)]
    existing = {
        entry["key"]: entry
        for entry in catalog["entries"]
        if entry["key"].startswith(MANAGED_PREFIX)
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
