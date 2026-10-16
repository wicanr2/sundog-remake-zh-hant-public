#!/usr/bin/env python3
"""由受版控來源產生 en／zh-Hans／ja 語言包草稿。

有 source 的項目以原版英文為錨；只有 remake 文案才以繁中為錨。網路翻譯只負責
初稿，輸出仍必須經 validate_language_packs.py 與人工語意覆核後才能標為 reviewed。
"""

from __future__ import annotations

import json
import time
import urllib.parse
import urllib.request
import sys
import subprocess
import html
import os
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
I18N = ROOT / "docs/i18n"


def machine_translate(text: str, source: str, target: str) -> str:
    if not text or source == target:
        return text
    query = urllib.parse.urlencode({"client": "gtx", "sl": source, "tl": target, "dt": "t", "q": text})
    request = urllib.request.Request(
        "https://translate.googleapis.com/translate_a/single?" + query,
        headers={"User-Agent": "SunDog-language-pack-builder/1"},
    )
    for attempt in range(5):
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                payload = json.load(response)
            return "".join(part[0] for part in payload[0] if part[0])
        except Exception:
            if attempt == 4:
                raise
            time.sleep(1 + attempt)
    raise AssertionError("unreachable")


def chrome_translate_batch(texts: list[str], source: str, target: str) -> list[str]:
    if not texts:
        return []
    # Google 的網頁翻譯會保留 XML 元素邊界。每個值各放一個 x 節點，避免自然語言重排跨越
    # catalog key；回傳必須仍能解析且節點數相同，否則失敗即關閉。
    payload = "".join(f"<x>{html.escape(value)}</x>" for value in texts)
    query = urllib.parse.urlencode({"sl": source, "tl": target, "text": payload, "op": "translate"})
    completed = subprocess.run(
        [
            "chromium-browser", "--headless", "--no-sandbox", "--disable-gpu",
            "--virtual-time-budget=10000", "--dump-dom",
            "https://translate.google.com/?" + query,
        ],
        stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, check=True, timeout=45,
    )
    pattern = re.compile(
        rf'<textarea[^>]*lang="{re.escape(target)}"[^>]*data-is-auto-expanding="true"[^>]*>(.*?)</textarea>',
        re.DOTALL,
    )
    match = pattern.search(completed.stdout)
    if not match:
        raise RuntimeError(f"Google Translate page has no {target} result textarea")
    translated = html.unescape(match.group(1))
    nodes = list(re.finditer(r"<x>(.*?)</x>", translated, re.DOTALL))
    outside = re.sub(r"<x>.*?</x>", "", translated, flags=re.DOTALL).strip()
    if len(texts) == 1 and len(nodes) == 1:
        # 單筆沒有跨 key 的可能；Google 偶爾把日文句尾移到 </x> 後，移除標籤即可
        # 依 DOM 原順序收回完整句子。多筆絕不走這條寬容路徑。
        value = translated.replace("<x>", "").replace("</x>", "").strip()
        if not value:
            raise RuntimeError("Google Translate returned an empty single item")
        return [value]
    if len(nodes) != len(texts) or outside:
        raise RuntimeError(
            f"Google Translate changed XML batch topology: nodes={len(nodes)}/{len(texts)}, outside={outside[:240]!r}"
        )
    result = [node.group(1).strip() for node in nodes]
    if any(not value for value in result):
        raise RuntimeError("Google Translate returned an empty batch item")
    return result


def translate_many(texts: list[str], source: str, target: str) -> list[str]:
    if os.environ.get("SUNDOG_TRANSLATE_CHROME") != "1":
        return [machine_translate(text, source, target) for text in texts]

    def translated(chunk: list[str]) -> list[str]:
        try:
            return chrome_translate_batch(chunk, source, target)
        except RuntimeError:
            if len(chunk) == 1:
                raise
            middle = len(chunk) // 2
            return translated(chunk[:middle]) + translated(chunk[middle:])

    output: list[str] = []
    for start in range(0, len(texts), 60):
        output.extend(translated(texts[start:start + 60]))
    return output


def translate_catalog(source_path: Path, locale: str) -> None:
    document = json.loads(source_path.read_text(encoding="utf-8"))
    pending: dict[str, list[tuple[dict, str]]] = {"en": [], "zh-TW": []}
    for entry in document["entries"]:
        if locale == "en" and entry.get("source"):
            entry["translation"] = entry["source"]
        else:
            anchor = entry.get("source") or entry["translation"]
            source_locale = "en" if entry.get("source") else "zh-TW"
            pending[source_locale].append((entry, anchor))
        entry["translationStatus"] = "ai-draft; requires one full review"
    for source_locale, pairs in pending.items():
        values = translate_many([anchor for _, anchor in pairs], source_locale, locale)
        for (entry, _), value in zip(pairs, values, strict=True):
            entry["translation"] = value
    document["locale"] = locale
    document["translationStatus"] = "ai-draft; requires one full review"
    stem = source_path.stem.replace("-zh-Hant", "")
    if stem == "apple2-text-catalog":
        out = I18N / f"apple2-text-{locale}.json"
    else:
        out = I18N / f"{stem}-{locale}.json"
    out.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def translate_dialog(locale: str) -> None:
    source_path = I18N / "atari-st-dialog-zh-Hant.json"
    document = json.loads(source_path.read_text(encoding="utf-8"))

    pending: list[dict] = []

    def visit(value):
        if isinstance(value, dict):
            if isinstance(value.get("source"), str) and isinstance(value.get("translation"), str):
                if locale == "en":
                    value["translation"] = value["source"]
                elif not re.sub(r"\{[0-9a-fA-F]{2}\}|\s+", "", value["source"]):
                    # 只有 VM 引用／結束標記，沒有自然語言可翻；原樣保留才不會遺失引用。
                    value["translation"] = value["source"]
                else:
                    pending.append(value)
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)

    visit(document)
    if pending:
        values = translate_many([entry["source"] for entry in pending], "en", locale)
        for entry, value in zip(pending, values, strict=True):
            entry["translation"] = value
    document["locale"] = locale
    document["translationStatus"] = "ai-draft; requires one full review"
    out = I18N / f"atari-st-dialog-{locale}.json"
    out.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> None:
    if "--japanese-dialog-only" in sys.argv:
        translate_dialog("ja")
        return
    if "--english-original-only" in sys.argv:
        apple = json.loads((I18N / "apple2-text-catalog.json").read_text(encoding="utf-8"))
        for entry in apple["entries"]:
            entry["translation"] = entry["source"]
            entry["translationStatus"] = "reviewed; original English source"
        apple["locale"] = "en"
        apple["translationStatus"] = "reviewed; original English source"
        (I18N / "apple2-text-en.json").write_text(
            json.dumps(apple, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        dialog = json.loads((I18N / "atari-st-dialog-zh-Hant.json").read_text(encoding="utf-8"))

        def original(value):
            if isinstance(value, dict):
                if isinstance(value.get("source"), str) and "translation" in value:
                    value["translation"] = value["source"]
                for child in value.values():
                    original(child)
            elif isinstance(value, list):
                for child in value:
                    original(child)

        original(dialog)
        dialog["locale"] = "en"
        dialog["translationStatus"] = "reviewed; original English source"
        (I18N / "atari-st-dialog-en.json").write_text(
            json.dumps(dialog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        return
    if "--simplified-only" in sys.argv:
        def simplified(text: str) -> str:
            completed = subprocess.run(
                ["opencc", "-c", "tw2sp.json"], input=text, text=True,
                stdout=subprocess.PIPE, check=True,
            )
            return completed.stdout
        for source_path, output_name in (
            (I18N / "sundog-ui-zh-Hant.json", "sundog-ui-zh-Hans.json"),
            (I18N / "apple2-text-catalog.json", "apple2-text-zh-Hans.json"),
            (I18N / "atari-st-dialog-zh-Hant.json", "atari-st-dialog-zh-Hans.json"),
        ):
            document = json.loads(source_path.read_text(encoding="utf-8"))

            def convert(value):
                if isinstance(value, dict):
                    for key, child in value.items():
                        if key == "translation" and isinstance(child, str):
                            value[key] = simplified(child)
                        else:
                            convert(child)
                elif isinstance(value, list):
                    for child in value:
                        convert(child)

            convert(document)
            document["locale"] = "zh-Hans"
            document["translationStatus"] = "ai-draft; requires one full review"
            (I18N / output_name).write_text(
                json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
            )
        return
    for locale in ("en", "ja"):
        translate_catalog(I18N / "sundog-ui-zh-Hant.json", locale)
        translate_catalog(I18N / "apple2-text-catalog.json", locale)
        translate_dialog(locale)


if __name__ == "__main__":
    main()
