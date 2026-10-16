#!/usr/bin/env python3
"""不依賴原始媒體的公開封包邊界與 manifest 單元測試。"""

from __future__ import annotations

import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

try:
    from tools import package_public
except ModuleNotFoundError:  # ``cd tools && python3 -m unittest discover``
    import package_public


class PackagePublicTests(unittest.TestCase):
    def run_package(self, root: Path, output: Path) -> int:
        with patch.object(sys, "argv", ["package_public", "--root", str(root), "--output", str(output)]):
            return package_public.main()

    def write_fixture(self, root: Path, relative: str, content: bytes = b"synthetic\n") -> None:
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)

    def make_public_root(self) -> Path:
        temp_dir = tempfile.mkdtemp()
        root = Path(temp_dir)
        self.addCleanup(lambda: __import__("shutil").rmtree(root))
        for relative in (
            "README.md",
            "LICENSE",
            "AGENTS.md",
            "docs/README.md",
            "internal/example.txt",
            "tools/example.py",
        ):
            self.write_fixture(root, relative, relative.encode("utf-8"))
        return root

    def test_includes_allowed_files_and_skips_excluded_paths(self) -> None:
        root = self.make_public_root()
        self.write_fixture(root, "docs/manual/atari-st-manual-ocr.txt")
        self.write_fixture(root, "assets/title/manual-cover.jpg")
        self.write_fixture(root, "docs/atari-st/parity-shots/cockpit.png")
        self.write_fixture(root, "docs/atari-st/parity-shots/README.md")
        self.write_fixture(root, "docs/screenshots/ground.png")
        self.write_fixture(root, "docs/art/screenshots/modern-apple-ground-v1.png")
        self.write_fixture(root, "docs/art/prototypes/atari-st-hd-direction-20261014.png")
        self.write_fixture(root, "docs/art/prototypes/atari-st-hd-shipboard-prototype-20261014.png")
        self.write_fixture(root, "docs/manual/keep.txt")
        self.write_fixture(root, "docs/workplace/ignored.txt")
        self.write_fixture(root, "tools/__pycache__/ignored.pyc")

        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "package"
            self.assertEqual(0, self.run_package(root, output))
            manifest = json.loads((output / "PACKAGE-MANIFEST.json").read_text(encoding="utf-8"))
            paths = {item["path"] for item in manifest["files"]}

        self.assertTrue({"README.md", "LICENSE", "AGENTS.md", "docs/README.md", "docs/manual/keep.txt", "internal/example.txt", "tools/example.py"} <= paths)
        self.assertNotIn("docs/manual/atari-st-manual-ocr.txt", paths)
        self.assertNotIn("assets/title/manual-cover.jpg", paths)
        self.assertNotIn("docs/atari-st/parity-shots/cockpit.png", paths)
        self.assertNotIn("docs/atari-st/parity-shots/README.md", paths)
        # README 與美術截圖自 2026-09-03 起同樣含原版像素，邊界跟對拍收據一樣。
        self.assertNotIn("docs/screenshots/ground.png", paths)
        self.assertNotIn("docs/art/screenshots/modern-apple-ground-v1.png", paths)
        self.assertNotIn("docs/art/prototypes/atari-st-hd-direction-20261014.png", paths)
        self.assertNotIn("docs/art/prototypes/atari-st-hd-shipboard-prototype-20261014.png", paths)
        self.assertNotIn("docs/workplace/ignored.txt", paths)
        self.assertNotIn("tools/__pycache__/ignored.pyc", paths)

    def test_every_forbidden_suffix_fails_closed(self) -> None:
        for suffix in sorted(package_public.FORBIDDEN_SUFFIXES):
            with self.subTest(suffix=suffix):
                root = self.make_public_root()
                self.write_fixture(root, f"docs/synthetic{suffix}")
                with tempfile.TemporaryDirectory() as temp_dir:
                    output = Path(temp_dir) / "package"
                    self.assertEqual(1, self.run_package(root, output))
                    self.assertFalse(output.exists())

    def test_every_forbidden_name_part_fails_closed(self) -> None:
        for name_part in package_public.FORBIDDEN_NAME_PARTS:
            with self.subTest(name_part=name_part):
                root = self.make_public_root()
                self.write_fixture(root, f"docs/{name_part}-synthetic.txt")
                with tempfile.TemporaryDirectory() as temp_dir:
                    output = Path(temp_dir) / "package"
                    self.assertEqual(1, self.run_package(root, output))
                    self.assertFalse(output.exists())

    def test_manifest_matches_packaged_files(self) -> None:
        root = self.make_public_root()
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "package"
            self.assertEqual(0, self.run_package(root, output))
            manifest = json.loads((output / "PACKAGE-MANIFEST.json").read_text(encoding="utf-8"))
            entries = manifest["files"]

            self.assertEqual(len(entries), manifest["fileCount"])
            self.assertEqual(
                {item["path"] for item in entries},
                {path.relative_to(output).as_posix() for path in output.rglob("*") if path.is_file() and path.name != "PACKAGE-MANIFEST.json"},
            )
            for item in entries:
                path = output / item["path"]
                payload = path.read_bytes()
                self.assertEqual(item["bytes"], len(payload), item["path"])
                self.assertEqual(item["sha256"], hashlib.sha256(payload).hexdigest(), item["path"])

    def test_existing_output_is_rejected(self) -> None:
        root = self.make_public_root()
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "package"
            output.mkdir()
            self.assertRaises(SystemExit, self.run_package, root, output)


if __name__ == "__main__":
    unittest.main()
