#!/usr/bin/env python3
"""驗證四語語言包的結構、key、格式參數與對話引用完全一致。"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
I18N = ROOT / "docs/i18n"
LOCALES = ("zh-Hant", "zh-Hans", "ja", "en")
PRINTF = re.compile(r"%(?:\[[0-9]+\])?[+#0 -]*(?:[0-9]+|\*)?(?:\.[0-9*]+)?[vTtbcdoOqxXUeEfFgGsp]")
BRACES = re.compile(r"\{[0-9a-fA-F]{2}\}")
MARKER_REFERENCES = {"{3f}"}
CJK = re.compile(r"[\u3400-\u9fff\u3040-\u30ff]")


def fail(message: str, errors: list[str]) -> None:
    errors.append(message)


def load_catalog(path: Path, expected_locale: str, errors: list[str]) -> dict[str, dict]:
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        fail(f"{path}: cannot load: {exc}", errors)
        return {}
    if document.get("locale") != expected_locale:
        fail(f"{path}: locale {document.get('locale')!r}, want {expected_locale!r}", errors)
    result: dict[str, dict] = {}
    for entry in document.get("entries", []):
        key, value = entry.get("key"), entry.get("translation")
        if not key or not isinstance(value, str) or not value:
            fail(f"{path}: invalid entry {entry!r}", errors)
            continue
        if key in result:
            fail(f"{path}: duplicate key {key}", errors)
        result[key] = entry
    return result


def tokens(value: str) -> tuple[tuple[str, ...], tuple[str, ...]]:
    return tuple(PRINTF.findall(value)), tuple(BRACES.findall(value))


def dialog_references(value: str) -> set[str]:
    return set(BRACES.findall(value)) - MARKER_REFERENCES


def walk_dialog(value, location: str, output: dict[str, tuple[str, str]], errors: list[str]) -> None:
    if isinstance(value, dict):
        if isinstance(value.get("source"), str) and isinstance(value.get("translation"), str):
            if location in output:
                fail(f"dialog duplicate location {location}", errors)
            output[location] = (value["source"], value["translation"])
        for key, child in value.items():
            walk_dialog(child, f"{location}/{key}", output, errors)
    elif isinstance(value, list):
        for index, child in enumerate(value):
            walk_dialog(child, f"{location}/{index}", output, errors)


def main() -> int:
    errors: list[str] = []
    families = {
        "ui": lambda locale: I18N / f"sundog-ui-{locale}.json",
        "apple2": lambda locale: I18N / ("apple2-text-catalog.json" if locale == "zh-Hant" else f"apple2-text-{locale}.json"),
    }
    for family, path_for in families.items():
        catalogs = {locale: load_catalog(path_for(locale), locale, errors) for locale in LOCALES}
        baseline = set(catalogs["zh-Hant"])
        for locale, entries in catalogs.items():
            if set(entries) != baseline:
                missing = sorted(baseline - set(entries))
                extra = sorted(set(entries) - baseline)
                fail(f"{family}/{locale}: missing={missing}, extra={extra}", errors)
            for key in baseline & set(entries):
                base = catalogs["zh-Hant"][key]["translation"]
                value = entries[key]["translation"]
                if tokens(value) != tokens(base):
                    fail(f"{family}/{locale}/{key}: format tokens {tokens(value)} != {tokens(base)}", errors)
                if "__SD" in value or "SUNDOG_DQ" in value:
                    fail(f"{family}/{locale}/{key}: translation batch marker remains", errors)
                if locale == "en" and CJK.search(value):
                    fail(f"{family}/{locale}/{key}: English translation contains CJK", errors)
                for opening, closing in (("(", ")"), ("（", "）"), ("[", "]")):
                    if value.count(opening) != value.count(closing):
                        fail(f"{family}/{locale}/{key}: unbalanced {opening}{closing}", errors)

    dialogs: dict[str, dict[str, tuple[str, str]]] = {}
    for locale in LOCALES:
        path = I18N / f"atari-st-dialog-{locale}.json"
        try:
            document = json.loads(path.read_text(encoding="utf-8"))
        except Exception as exc:
            fail(f"{path}: cannot load: {exc}", errors)
            continue
        if document.get("locale") != locale:
            fail(f"{path}: locale {document.get('locale')!r}, want {locale!r}", errors)
        entries: dict[str, tuple[str, str]] = {}
        walk_dialog(document, "", entries, errors)
        dialogs[locale] = entries
    if "zh-Hant" in dialogs:
        baseline = dialogs["zh-Hant"]
        for locale, entries in dialogs.items():
            if set(entries) != set(baseline):
                fail(f"dialog/{locale}: translation topology differs", errors)
                continue
            for location, (source, translation) in entries.items():
                base_source = baseline[location][0]
                if source != base_source:
                    fail(f"dialog/{locale}{location}: source drift", errors)
                if dialog_references(translation) != dialog_references(source):
                    fail(f"dialog/{locale}{location}: reference tokens drift", errors)
                if "__SD" in translation or "SUNDOG_DQ" in translation:
                    fail(f"dialog/{locale}{location}: translation batch marker remains", errors)
                if locale == "en" and CJK.search(translation):
                    fail(f"dialog/{locale}{location}: English translation contains CJK", errors)

    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("language packs: 4 locales, catalog keys and dialog references verified")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
