#!/usr/bin/env python3
"""封裝工具的單元測試。不編譯、不打真的套件，只驗版面規則。"""

from __future__ import annotations

import struct
import tempfile
import unittest
import zipfile
from pathlib import Path

from package_release import (
    SYSTEM_DLLS,
    UTF8_NAME_FLAG,
    ReleaseError,
    force_utf8_flag,
    verify_zip_utf8,
    zip_tree,
)


def build_tree(root: Path) -> None:
    (root / "docs").mkdir(parents=True)
    (root / "README.md").write_text("readme", encoding="utf-8")
    # 中文檔名與純 ASCII 檔名都要驗到：Python 只會替前者自動補旗標。
    (root / "操作說明.md").write_text("keys", encoding="utf-8")
    (root / "docs" / "catalog.json").write_text("{}", encoding="utf-8")
    binary = root / "game.exe"
    binary.write_bytes(b"MZ")
    binary.chmod(0o755)


class ZipFlagTests(unittest.TestCase):
    def test_every_entry_declares_utf8_in_both_headers(self) -> None:
        with tempfile.TemporaryDirectory() as name:
            temp = Path(name)
            root = temp / "payload"
            root.mkdir()
            build_tree(root)
            archive = temp / "out.zip"
            zip_tree(root, archive, "payload")

            data = archive.read_bytes()
            with zipfile.ZipFile(archive) as bundle:
                self.assertTrue(bundle.infolist())
                for info in bundle.infolist():
                    self.assertTrue(info.flag_bits & UTF8_NAME_FLAG, info.filename)
                    local = struct.unpack_from("<H", data, info.header_offset + 6)[0]
                    self.assertTrue(local & UTF8_NAME_FLAG, info.filename)

    def test_an_ascii_only_name_also_gets_the_flag(self) -> None:
        # 這是 Python 自己不會做的那一半：ASCII 檔名預設不帶旗標。
        with tempfile.TemporaryDirectory() as name:
            temp = Path(name)
            archive = temp / "plain.zip"
            with zipfile.ZipFile(archive, "w") as bundle:
                bundle.writestr("plain.txt", "x")
            with zipfile.ZipFile(archive) as bundle:
                self.assertFalse(bundle.infolist()[0].flag_bits & UTF8_NAME_FLAG)
            force_utf8_flag(archive)
            with zipfile.ZipFile(archive) as bundle:
                self.assertTrue(bundle.infolist()[0].flag_bits & UTF8_NAME_FLAG)

    def test_the_archive_still_reads_back_after_patching(self) -> None:
        with tempfile.TemporaryDirectory() as name:
            temp = Path(name)
            root = temp / "payload"
            root.mkdir()
            build_tree(root)
            archive = temp / "out.zip"
            zip_tree(root, archive, "payload")
            with zipfile.ZipFile(archive) as bundle:
                self.assertIsNone(bundle.testzip())
                self.assertEqual(bundle.read("payload/操作說明.md"), b"keys")

    def test_the_executable_bit_survives(self) -> None:
        with tempfile.TemporaryDirectory() as name:
            temp = Path(name)
            root = temp / "payload"
            root.mkdir()
            build_tree(root)
            archive = temp / "out.zip"
            zip_tree(root, archive, "payload")
            with zipfile.ZipFile(archive) as bundle:
                info = bundle.getinfo("payload/game.exe")
            self.assertTrue((info.external_attr >> 16) & 0o111)

    def test_verification_rejects_an_archive_without_the_flag(self) -> None:
        with tempfile.TemporaryDirectory() as name:
            archive = Path(name) / "plain.zip"
            with zipfile.ZipFile(archive, "w") as bundle:
                bundle.writestr("plain.txt", "x")
            with self.assertRaises(ReleaseError):
                verify_zip_utf8(archive)


class SystemDllTests(unittest.TestCase):
    def test_kernel32_counts_as_a_system_dll(self) -> None:
        # 純 Go 的 Windows 建置只會匯入 kernel32；名單是用來擋「其實需要卻沒包」的。
        self.assertIn("kernel32.dll", SYSTEM_DLLS)

    def test_the_allowlist_is_lowercase(self) -> None:
        for name in SYSTEM_DLLS:
            self.assertEqual(name, name.lower())


if __name__ == "__main__":
    unittest.main()
