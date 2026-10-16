from __future__ import annotations

import hashlib
import unittest

from validate_apple2_status_label_evidence import validate


class StatusLabelEvidenceValidationTest(unittest.TestCase):
    def make_document(self, disk1: bytes, disk2: bytes) -> dict:
        return {
            "input": {
                "disk1": {"size": len(disk1), "sha256": hashlib.sha256(disk1).hexdigest()},
                "disk2": {"size": len(disk2), "sha256": hashlib.sha256(disk2).hexdigest()},
            },
            "rawLabelAnchors": [
                {
                    "key": "apple2.startup.sleep",
                    "value": "Sleep",
                    "translation": "睡眠",
                    "rawOffset": "0x4",
                    "rawHex": "536c656570",
                },
                {
                    "key": "apple2.startup.vigor",
                    "value": "Vigor",
                    "translation": "活力",
                    "rawOffset": "0x10",
                    "rawHex": "5669676f72",
                },
            ],
        }

    def test_accepts_exact_dual_disk_bytes(self) -> None:
        disk1 = b"\x00" * 4 + b"Sleep" + b"\x00" * 7 + b"Vigor" + b"\x00" * 8
        disk2 = bytes(disk1)
        self.assertEqual([], validate(disk1, disk2, self.make_document(disk1, disk2)))

    def test_rejects_disk2_byte_drift(self) -> None:
        disk1 = b"\x00" * 4 + b"Sleep" + b"\x00" * 7 + b"Vigor" + b"\x00" * 8
        disk2 = bytearray(disk1)
        disk2[0x10] = ord("X")
        document = self.make_document(disk1, bytes(disk2))
        self.assertIn("anchor apple2.startup.vigor disk2 raw bytes mismatch", validate(disk1, bytes(disk2), document))

    def test_rejects_duplicate_key(self) -> None:
        disk = b"\x00" * 4 + b"Sleep" + b"\x00" * 7 + b"Vigor" + b"\x00" * 8
        document = self.make_document(disk, disk)
        document["rawLabelAnchors"][1]["key"] = "apple2.startup.sleep"
        self.assertIn("anchor 1 has missing or duplicate key: 'apple2.startup.sleep'", validate(disk, disk, document))


if __name__ == "__main__":
    unittest.main()
