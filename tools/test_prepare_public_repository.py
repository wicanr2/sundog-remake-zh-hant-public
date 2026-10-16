"""公開範本不得帶入未追蹤的本機檔案或 symlink 目標。"""

import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import prepare_public_repository as publication


class PublicTemplateBoundaryTests(unittest.TestCase):
    def fixture(self, root):
        tracked = set(publication.TOP) | publication.DOCS | {"tools/public-source/README.md"}
        for name in tracked:
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("tracked fixture\n")
        return ("\0".join(sorted(tracked)) + "\0").encode()

    def test_untracked_template_is_excluded(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "source"
            tracked = self.fixture(root)
            (root / "tools/public-source/private-note.txt").write_text("local only\n")
            output = Path(directory) / "public"
            with patch.object(publication.subprocess, "check_output", return_value=tracked), patch.object(publication, "SHOTS", {}), patch.object(publication, "LANGUAGE_SHOTS", {}):
                publication.prepare(root, output, "v.1.2.0-20261016")
            self.assertFalse((output / "private-note.txt").exists())
            self.assertFalse((output / "tools/public-source/private-note.txt").exists())
            self.assertEqual("tracked fixture\n", (output / "README.md").read_text())

    def test_tracked_template_symlink_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "source"
            tracked = self.fixture(root)
            outside = Path(directory) / "outside.txt"
            outside.write_text("outside the source tree\n")
            template = root / "tools/public-source/README.md"
            template.unlink()
            template.symlink_to(outside)
            with patch.object(publication.subprocess, "check_output", return_value=tracked), patch.object(publication, "SHOTS", {}), patch.object(publication, "LANGUAGE_SHOTS", {}):
                with self.assertRaisesRegex(ValueError, "symlink"):
                    publication.prepare(root, Path(directory) / "public", "v.1.2.0-20261016")


    def test_changed_localized_screenshot_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "source"
            tracked = self.fixture(root)
            name = "docs/screenshots/hd-warehouse-en-20261016.png"
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b"unreviewed replacement")
            tracked += (name + "\0").encode()
            with patch.object(publication.subprocess, "check_output", return_value=tracked), patch.object(publication, "SHOTS", {}), patch.object(publication, "LANGUAGE_SHOTS", {"en": "0" * 64}):
                with self.assertRaisesRegex(ValueError, "HD screenshot changed"):
                    publication.prepare(root, Path(directory) / "public", "v.1.2.0-20261016")


if __name__ == "__main__":
    unittest.main()
