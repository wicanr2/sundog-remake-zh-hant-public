"""韓文唯一來源與完整產物的同步、重複 key 防護。"""

import shutil
import tempfile
import unittest
from pathlib import Path

from build_korean_language_pack import build

ROOT = Path(__file__).resolve().parents[1]


class KoreanLanguagePackTests(unittest.TestCase):
    def test_real_catalogs_match_canonical_tsv(self):
        build(ROOT, check=True)

    def test_duplicate_tsv_identity_is_rejected_before_output(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            directory = root / "docs/i18n"
            directory.mkdir(parents=True)
            source = ROOT / "docs/i18n/ko.tsv"
            target = directory / "ko.tsv"
            shutil.copyfile(source, target)
            with target.open("a", encoding="utf-8") as stream:
                stream.write(source.read_text().splitlines()[1] + "\n")
            with self.assertRaisesRegex(ValueError, "duplicate"):
                build(root)
            self.assertEqual(["ko.tsv"], [p.name for p in directory.iterdir()])
