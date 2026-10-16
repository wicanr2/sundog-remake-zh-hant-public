#!/usr/bin/env python3
"""驗證 Markdown 手冊的頁面與語言區段覆蓋。"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path


def parse_page_range(value: str) -> list[int]:
    match = re.fullmatch(r"(\d+)-(\d+)", value)
    if not match:
        raise ValueError(f"invalid page range: {value!r}")
    start, end = (int(part) for part in match.groups())
    if start > end:
        raise ValueError(f"reversed page range: {value!r}")
    return list(range(start, end + 1))


def sha256(value: str) -> bool:
    return bool(re.fullmatch(r"[0-9a-f]{64}", value))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("manual", type=Path)
    parser.add_argument("evidence", type=Path)
    args = parser.parse_args()

    errors: list[str] = []
    manual = args.manual.read_text(encoding="utf-8")
    evidence = json.loads(args.evidence.read_text(encoding="utf-8"))

    source = evidence.get("source", {})
    if source.get("pageCount") != 22:
        errors.append(f"source pageCount={source.get('pageCount')!r}, want 22")
    if not sha256(source.get("sha256", "")):
        errors.append("source sha256 is missing or malformed")

    groups = evidence.get("languageGroups", [])
    covered: list[int] = []
    for group in groups:
        try:
            covered.extend(parse_page_range(group["pages"]))
        except (KeyError, ValueError) as exc:
            errors.append(str(exc))
    if covered != list(range(1, 23)):
        errors.append(f"language groups cover {covered!r}, want pages 1..22 exactly once")

    canonical = evidence.get("canonicalTranslation", {})
    if canonical.get("target") != "zh-Hant":
        errors.append("canonical translation target is not zh-Hant")
    if canonical.get("uniqueContentPages") != "1-10":
        errors.append("unique content pages must be 1-10")
    if canonical.get("duplicateLanguagePages") != "11-22":
        errors.append("duplicate language pages must be 11-22")

    required_headings = [
        "## 第 1 頁",
        "## 第 2 頁",
        "## 第 3–4 頁",
        "## 第 4 頁",
        "## 第 5 頁",
        "## 第 6 頁",
        "## 第 7 頁",
        "## 第 8 頁",
        "## 第 9 頁",
        "## 第 10 頁",
    ]
    for heading in required_headings:
        if heading not in manual:
            errors.append(f"missing manual heading: {heading}")
    for page in range(11, 23):
        if not re.search(rf"\|\s*{page}\s*\|", manual):
            errors.append(f"missing language mapping row for page {page}")
        if f"## 第 {page} 頁" not in manual:
            errors.append(f"missing canonical Markdown page node for page {page}")

    for term in (
        "TRANSFER",
        "WITHDRAW CASH",
        "DEPOSIT CASH",
        "Library Disk",
        # 星系名稱以磁碟上的世界表為準；手冊正文寫作「瓊德(Jondd)」。
        "Jondd",
        "cryogens",
    ):
        if term not in manual:
            errors.append(f"missing required source term: {term}")

    for page_key in ("page03PngSha256", "page04PngSha256"):
        value = evidence.get("page3to4HighResolutionReview", {}).get("renderedPageHashes", {}).get(page_key, "")
        if not sha256(value):
            errors.append(f"high-resolution evidence hash is missing or malformed: {page_key}")
    for page_key in ("page02PngSha256", "page10PngSha256"):
        value = evidence.get("page2And10LegalReview", {}).get("renderedPageHashes", {}).get(page_key, "")
        if not sha256(value):
            errors.append(f"legal-page evidence hash is missing or malformed: {page_key}")

    result = {
        "status": "pass" if not errors else "fail",
        "errors": errors,
        "sourcePageCount": source.get("pageCount"),
        "manual": str(args.manual),
        "evidence": str(args.evidence),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
