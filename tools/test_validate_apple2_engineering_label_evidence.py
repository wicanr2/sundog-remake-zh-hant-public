from __future__ import annotations

import hashlib
import unittest

from validate_apple2_engineering_label_evidence import validate


class EngineeringLabelEvidenceValidationTest(unittest.TestCase):
    def make_document(self, disk: bytes, start: int, segment: bytes, label: bytes, label_offset: int) -> dict:
        return {
            "input": {"disk2": {"size": len(disk), "sha256": hashlib.sha256(disk).hexdigest()}},
            "rawTableSegment": {
                "offset": hex(start),
                "length": len(segment),
                "sha256": hashlib.sha256(segment).hexdigest(),
                "rawHex": segment.hex(),
                "label": {
                    "value": label.decode("ascii"),
                    "offsetWithinSegment": label_offset,
                    "rawOffset": hex(start + label_offset),
                    "rawHex": label.hex(),
                },
            },
        }

    def test_accepts_exact_segment_and_label(self) -> None:
        label = b"Sub-c Engines"
        segment = b"\xf4\xb2\x01" + label + b"\xd7\xaa"
        disk = b"\x00" * 32 + segment + b"\x00" * 32
        document = self.make_document(disk, 32, segment, label, 3)
        self.assertEqual([], validate(disk, document))

    def test_rejects_label_offset_drift(self) -> None:
        label = b"Sub-c Engines"
        segment = b"\xf4\xb2\x01" + label + b"\xd7\xaa"
        disk = b"\x00" * 32 + segment + b"\x00" * 32
        document = self.make_document(disk, 32, segment, label, 3)
        document["rawTableSegment"]["label"]["rawOffset"] = "0x0"
        self.assertIn("label rawOffset does not match segment offset", validate(disk, document))


if __name__ == "__main__":
    unittest.main()
