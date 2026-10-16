#!/usr/bin/env python3
"""不依賴原始媒體的 Apple II DSK 輸入契約測試。"""

from __future__ import annotations

import tempfile
import unittest
import zipfile
import hashlib
from pathlib import Path

try:
    from tools.apple2_dsk_inventory import EXPECTED_SIZE, load_input
except ModuleNotFoundError:  # ``cd tools && python3 -m unittest discover``
    from apple2_dsk_inventory import EXPECTED_SIZE, load_input


class Apple2DSKInventoryTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.data = bytearray(EXPECTED_SIZE)
        self.data[0x9E : 0x9E + len(b"MUST BOOT FROM SLOT 6")] = b"MUST BOOT FROM SLOT 6"

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def test_raw_disk_input(self) -> None:
        path = self.root / "SUNDOG1.DSK"
        path.write_bytes(self.data)

        kind, input_sha, source_files, disks = load_input(path)

        self.assertEqual(kind, "disk")
        self.assertIsNotNone(input_sha)
        self.assertEqual([name for name, _ in disks], ["SUNDOG1.DSK"])
        self.assertEqual(source_files[0]["size"], EXPECTED_SIZE)
        self.assertEqual(source_files[0]["sha256"], input_sha)

    def test_directory_input_reads_only_direct_dsk_files(self) -> None:
        directory = self.root / "disks"
        directory.mkdir()
        (directory / "disk-2.dsk").write_bytes(self.data)
        (directory / "notes.txt").write_text("not a disk\n", encoding="utf-8")
        nested = directory / "nested"
        nested.mkdir()
        (nested / "ignored.dsk").write_bytes(self.data)

        kind, input_sha, source_files, disks = load_input(directory)

        self.assertEqual(kind, "directory")
        self.assertIsNone(input_sha)
        self.assertEqual([name for name, _ in disks], ["disk-2.dsk"])
        self.assertEqual([item["name"] for item in source_files], ["disk-2.dsk"])

    def test_zip_input_preserves_archive_hash_and_member_name(self) -> None:
        archive_path = self.root / "source.zip"
        with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_STORED) as archive:
            archive.writestr("SUNDOG2.DSK", self.data)

        kind, input_sha, source_files, disks = load_input(archive_path)

        self.assertEqual(kind, "zip")
        self.assertEqual(input_sha, hashlib.sha256(archive_path.read_bytes()).hexdigest())
        self.assertEqual([name for name, _ in disks], ["SUNDOG2.DSK"])
        self.assertEqual(source_files[0]["name"], "SUNDOG2.DSK")


if __name__ == "__main__":
    unittest.main()
