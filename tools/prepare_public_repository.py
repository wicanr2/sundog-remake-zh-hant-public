#!/usr/bin/env python3
"""從已追蹤的來源建立全新公開工作樹；不複製 Git 歷史或私人研究資料。"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shutil
import subprocess
from pathlib import Path

from package_public import is_forbidden, iter_public_files

TEMPLATES = Path("tools/public-source")
TOP = {".gitignore", "LICENSE", "go.mod", "go.sum"}
DIRECTORIES = {"assets", "cmd", "internal", "docker", "tools"}
DOCS = {"docs/manual/remake-controls-zh-Hant.md"}
SHOTS = {
    "shipboard": "5a8d8a1aad3f26ff44c67648ffe0b93cafc92200fa0bf4924c085ae4d6f52c9e",
    "ground": "6108fc6c52f1098e5c94da43887753e5585957fc5a381a9a43e27b821320c6f7",
    "system": "596504b0d90e1c26b1047918e1993be67c5ad0fc06e4c67a9bc39eca683f79a7",
    "engineering": "8c7694bf617947de5485721a7f51512a7c9518e3f3bed21bb84467ba1439e622",
    "stores": "5f5109f0fceee4a8b9bb0e08529eba570aa8c0a5606b9c220486591b902154e3",
}


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def prepare(root: Path, output: Path) -> dict:
    if output.exists():
        raise ValueError(f"output already exists: {output}")
    tracked = set(subprocess.check_output(
        ["git", "-c", f"safe.directory={root}", "-C", str(root), "ls-files", "-z"],
    ).decode().split("\0"))
    # Ordinary tests read independently decoded facts and controlled oracle
    # answers. Keep those exact JSON receipts, without the private image tree.
    required_docs = set(DOCS)
    for name in sorted(tracked):
        if name.endswith("_test.go"):
            text = (root / name).read_text()
            required_docs.update(re.findall(r'"(?:\.\./)*(docs/[^"\n]+\.json)"', text))
    selected = []
    for path in iter_public_files(root):
        name = path.relative_to(root).as_posix()
        if name not in tracked:
            continue
        if name.startswith("assets/title/"):
            continue
        if name in TOP or name.split("/")[0] in DIRECTORIES or name in required_docs or name.startswith("docs/i18n/"):
            error = is_forbidden(Path(name))
            if error:
                raise ValueError(error)
            if path.is_symlink():
                raise ValueError(f"symlink is not publishable: {name}")
            selected.append(path)
    for name in TOP | required_docs:
        if name not in {p.relative_to(root).as_posix() for p in selected}:
            raise ValueError(f"required tracked source missing: {name}")
    # These reviewed screenshots show independently drawn HD art. The source
    # directory stays excluded; only these five exact files are relocated.
    screenshots = []
    for scene in SHOTS:
        name = f"docs/atari-st/parity-shots/hd-full-{scene}-20261015.png"
        path = root / name
        if name not in tracked or not path.is_file() or path.is_symlink():
            raise ValueError(f"reviewed HD screenshot missing: {name}")
        if digest(path) != SHOTS[scene]:
            raise ValueError(f"HD screenshot changed; review before publication: {name}")
        screenshots.append((path, f"docs/screenshots/hd-{scene}.png"))
    output.mkdir(parents=True)
    for path in selected:
        destination = output / path.relative_to(root)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(path, destination)
    for path in (root / TEMPLATES).rglob("*"):
        if path.is_file():
            destination = output / path.relative_to(root / TEMPLATES)
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, destination)
    for source, name in screenshots:
        destination = output / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, destination)
    document = {
        "schema": 1,
        "boundary": "Fresh source snapshot; no original Git objects, disks, scans, original pixels or sounds. Private oracle images and full local packages excluded.",
        "gameVersion": "v.1.1.0-20261015",
        "screenshots": [{"path": name, "sha256": digest(source), "kind": "independently drawn HD; normal player capture", "captureDate": "2026-10-15"} for source, name in screenshots],
        "files": [{"path": p.relative_to(output).as_posix(), "bytes": p.stat().st_size, "sha256": digest(p)} for p in sorted(output.rglob("*")) if p.is_file()],
    }
    (output / "PUBLIC-SOURCE-MANIFEST.json").write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n")
    return {"status": "pass", "files": len(document["files"]), "output": str(output)}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(prepare(args.root.resolve(), args.output.resolve()), ensure_ascii=False))
