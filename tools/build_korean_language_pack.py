#!/usr/bin/env python3
"""由 UTF-8 ko.tsv 產生既有 runtime 格式的韓文 JSON；不改變來源 identity。"""

from __future__ import annotations

import argparse
import copy
import csv
import json
from pathlib import Path


def dialog_entries(value, path=""):
    if isinstance(value, dict):
        if isinstance(value.get("source"), str) and isinstance(value.get("translation"), str):
            yield path, value
        for key, child in value.items():
            yield from dialog_entries(child, path + "/" + key)
    elif isinstance(value, list):
        for index, child in enumerate(value):
            yield from dialog_entries(child, path + "/" + str(index))


def build(root: Path, check: bool = False) -> None:
    directory = root / "docs/i18n"
    rows = {}
    with (directory / "ko.tsv").open(encoding="utf-8", newline="") as stream:
        reader = csv.DictReader(stream, delimiter="\t")
        if reader.fieldnames != ["family", "key", "text_json"]:
            raise ValueError("ko.tsv requires family/key/text_json columns")
        for line, row in enumerate(reader, 2):
            key = (row["family"], row["key"])
            if None in row or key in rows:
                raise ValueError(f"duplicate or malformed TSV row {line}: {key}")
            text = json.loads(row["text_json"])
            if not isinstance(text, str) or not text:
                raise ValueError(f"empty/non-string translation at {line}: {key}")
            rows[key] = text
    used = set()
    families = [("ui", "sundog-ui-en.json", "sundog-ui-ko.json"),
                ("apple2", "apple2-text-en.json", "apple2-text-ko.json"),
                ("dialog", "atari-st-dialog-en.json", "atari-st-dialog-ko.json")]
    for family, source, destination in families:
        document = copy.deepcopy(json.loads((directory / source).read_text()))
        document["locale"] = "ko"
        document["translationStatus"] = "AI translation; structurally verified; native editorial review pending"
        document["translationSource"] = "docs/i18n/ko.tsv"
        entries = dialog_entries(document) if family == "dialog" else ((entry["key"], entry) for entry in document["entries"])
        for key, entry in entries:
            identity = (family, key)
            if identity not in rows:
                raise ValueError(f"missing translation: {identity}")
            entry["translation"] = rows[identity]
            if "translationStatus" in entry:
                entry["translationStatus"] = document["translationStatus"]
            used.add(identity)
        result = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
        target = directory / destination
        if check:
            if not target.is_file() or target.read_text() != result:
                raise ValueError(f"generated Korean catalog drift: {destination}")
        else:
            target.write_text(result, encoding="utf-8")
    if set(rows) != used:
        raise ValueError(f"orphan TSV entries: {sorted(set(rows) - used)}")
    print(f"Korean catalogs: {len(used)} stable entries; {'verified' if check else 'generated'}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    build(args.root, args.check)
