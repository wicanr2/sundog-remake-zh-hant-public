#!/usr/bin/env python3
"""建立不含原始媒體的 SunDog 公開研究／remake 封包。"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import sys
from pathlib import Path


TOP_LEVEL_FILES = {
    ".gitignore",
    "LICENSE",
    "AGENTS.md",
    "HANDOFF.md",
    "PLAN.md",
    "README.md",
    "RESEARCH-LOG.md",
    "VERIFICATION-MATRIX.md",
    "go.mod",
    "go.sum",
}
TOP_LEVEL_DIRECTORIES = {"assets", "cmd", "docker", "docs", "internal", "tools"}
SKIP_RELATIVE_PATHS = {
    Path("docs/manual/atari-st-manual-ocr.txt"),
    Path("assets/title/manual-cover.jpg"),
    Path("docs/art/prototypes/atari-st-hd-direction-20261014.png"),
}
# 這三個目錄的截圖含解自玩家自備磁碟的原版像素，見各自的 README。
# 2026-10-06 使用者要求保留截圖，最後維持私人 repo；一般研究封包仍排除這些目錄。
SKIP_RELATIVE_DIRECTORIES = (
    Path("docs/art/prototypes"),
    Path("docs/atari-st/parity-shots"),
    Path("docs/screenshots"),
    Path("docs/art/screenshots"),
)
FORBIDDEN_SUFFIXES = {
    ".bin",
    ".dsk",
    ".img",
    ".pdf",
    ".rar",
    ".rom",
    ".st",
    ".stx",
    ".zip",
}
FORBIDDEN_NAME_PARTS = (
    "Sundog-Frozen-Legacy",
    "SUNDOG1.DSK",
    "SUNDOG2.DSK",
    "atari-st-manual-ocr",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def is_forbidden(path: Path) -> str | None:
    if path.suffix.lower() in FORBIDDEN_SUFFIXES:
        return f"forbidden suffix: {path}"
    if any(part in path.name for part in FORBIDDEN_NAME_PARTS):
        return f"forbidden source name: {path}"
    return None


def iter_public_files(root: Path) -> list[Path]:
    candidates: list[Path] = []
    for name in sorted(TOP_LEVEL_FILES):
        path = root / name
        if path.is_file():
            candidates.append(path)
    for name in sorted(TOP_LEVEL_DIRECTORIES):
        directory = root / name
        if not directory.is_dir():
            continue
        for path in sorted(directory.rglob("*")):
            if not path.is_file():
                continue
            relative = path.relative_to(root)
            if relative in SKIP_RELATIVE_PATHS:
                continue
            if any(relative.is_relative_to(skipped) for skipped in SKIP_RELATIVE_DIRECTORIES):
                continue
            if any(part in {"workplace", ".git", "__pycache__"} for part in relative.parts):
                continue
            candidates.append(path)
    return candidates


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    root = args.root.resolve()
    output = args.output.resolve()
    if not root.is_dir():
        parser.error(f"root is not a directory: {root}")
    if output.exists():
        parser.error(f"output already exists; use a fresh directory: {output}")

    files = iter_public_files(root)
    errors: list[str] = []
    for source in files:
        relative = source.relative_to(root)
        error = is_forbidden(relative)
        if error:
            errors.append(error)
    if errors:
        print(json.dumps({"status": "fail", "errors": errors}, ensure_ascii=False, indent=2))
        return 1

    output.mkdir(parents=True)
    manifest: list[dict[str, object]] = []
    for source in files:
        relative = source.relative_to(root)
        destination = output / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, destination)
        manifest.append(
            {
                "path": relative.as_posix(),
                "bytes": source.stat().st_size,
                "sha256": sha256(source),
            }
        )

    violations: list[str] = []
    for path in output.rglob("*"):
        if path.is_file():
            error = is_forbidden(path.relative_to(output))
            if error:
                violations.append(error)
    if violations:
        print(json.dumps({"status": "fail", "errors": violations}, ensure_ascii=False, indent=2))
        return 1

    manifest_document = {
        "schema": 1,
        "status": "pass",
        "sourceBoundary": "original DSK/STX/ST/PDF/ROM/archive/OCR inputs remain outside this package",
        "fileCount": len(manifest),
        "files": manifest,
    }
    (output / "PACKAGE-MANIFEST.json").write_text(
        json.dumps(manifest_document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(json.dumps({"status": "pass", "fileCount": len(manifest), "output": str(output)}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
