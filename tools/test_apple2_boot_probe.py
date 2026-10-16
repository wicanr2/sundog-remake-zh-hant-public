#!/usr/bin/env python3
import unittest

try:
    from tools.apple2_boot_probe import (
        BOOT_PHYSICAL_SECTORS,
        DISK_SIZE,
        extract_boot,
        marker_evidence,
        source_raw_offset,
    )
except ModuleNotFoundError:  # ``cd tools && python3 -m unittest discover``
    from apple2_boot_probe import (
        BOOT_PHYSICAL_SECTORS,
        DISK_SIZE,
        extract_boot,
        marker_evidence,
        source_raw_offset,
    )


class Apple2BootProbeTests(unittest.TestCase):
    def make_disk(self) -> bytes:
        data = bytearray(DISK_SIZE)
        for physical, value in enumerate(range(16)):
            start = physical * 256
            data[start : start + 256] = bytes([value]) * 256
        data[0x9E : 0x9E + len(b"MUST BOOT FROM SLOT 6")] = b"MUST BOOT FROM SLOT 6"
        return bytes(data)

    def test_extract_preserves_candidate_physical_sector_order(self):
        boot = extract_boot(self.make_disk())
        self.assertEqual(
            [boot[index * 256] for index in range(4)], list(BOOT_PHYSICAL_SECTORS)
        )
        self.assertEqual(source_raw_offset(0x100), 0x0E00)
        self.assertEqual(source_raw_offset(0x2FF), 0x0DFF)

    def test_marker_keeps_boot_and_source_offsets(self):
        markers = marker_evidence(extract_boot(self.make_disk()))
        boot_marker = next(item for item in markers if item["marker"] == "MUST BOOT FROM SLOT 6")
        self.assertEqual(boot_marker["bootBufferOffset"], "0x009e")
        self.assertEqual(boot_marker["sourceRawOffset"], "0x009e")
        self.assertEqual(boot_marker["evidenceLevel"], "proven-bytes")


if __name__ == "__main__":
    unittest.main()
