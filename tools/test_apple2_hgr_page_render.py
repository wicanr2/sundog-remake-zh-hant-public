from __future__ import annotations

import hashlib
import json
import struct
import tempfile
import unittest
from pathlib import Path

from apple2_hgr_page_render import HGR_PAGE_SIZE, build_manifest, validate_page
from apple2_hgr_probe import render_hgr, write_png


class HGRPageRenderTest(unittest.TestCase):
    def test_rejects_non_page_length(self) -> None:
        with self.assertRaisesRegex(ValueError, "want exactly"):
            validate_page(b"\x00" * (HGR_PAGE_SIZE - 1))

    def test_manifest_preserves_page_hash_and_logical_size(self) -> None:
        page = bytes(index % 256 for index in range(HGR_PAGE_SIZE))
        manifest = build_manifest(Path("shipboard-page-1.bin"), page, "shipboard.png", 2)
        self.assertEqual(hashlib.sha256(page).hexdigest(), manifest["source"]["sha256"])
        self.assertEqual({"width": 280, "height": 192, "scale": 2}, manifest["logicalImage"])
        self.assertEqual("shipboard.png", manifest["output"])

    def test_rendered_png_has_scaled_hgr_dimensions(self) -> None:
        page = bytes(HGR_PAGE_SIZE)
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "page.png"
            write_png(output, render_hgr(page), 2)
            payload = output.read_bytes()
        self.assertEqual(b"\x89PNG\r\n\x1a\n", payload[:8])
        self.assertEqual((560, 384), struct.unpack(">II", payload[16:24]))

    def test_manifest_is_json_serializable(self) -> None:
        page = b"\x00" * HGR_PAGE_SIZE
        encoded = json.dumps(build_manifest(Path("page.bin"), page, "page.png", 1), ensure_ascii=False)
        self.assertIn("Apple II HGR", encoded)


if __name__ == "__main__":
    unittest.main()
