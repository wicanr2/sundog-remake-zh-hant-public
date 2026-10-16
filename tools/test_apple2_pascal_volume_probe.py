#!/usr/bin/env python3
import unittest

try:
    from tools.apple2_pascal_volume_probe import (
        BLOCK_SIZE,
        SECTOR_ORDER_CANDIDATES,
        parse_pascal_directory,
        probe_disk,
        read_block,
    )
except ModuleNotFoundError:  # ``cd tools && python3 -m unittest discover``
    from apple2_pascal_volume_probe import (
        BLOCK_SIZE,
        SECTOR_ORDER_CANDIDATES,
        parse_pascal_directory,
        probe_disk,
        read_block,
    )


class Apple2PascalVolumeProbeTests(unittest.TestCase):
    def make_pascal_disk(self) -> bytes:
        data = bytearray(35 * 16 * 256)
        order = SECTOR_ORDER_CANDIDATES["apple-pascal-2-ascending"]
        directory = bytearray(4 * BLOCK_SIZE)
        directory[0:2] = (0).to_bytes(2, "little")
        directory[2:4] = (6).to_bytes(2, "little")
        directory[4:6] = (0).to_bytes(2, "little")
        directory[6] = 4
        directory[7:11] = b"TEST"
        directory[0x0E:0x10] = (280).to_bytes(2, "little")
        directory[0x10:0x12] = (1).to_bytes(2, "little")
        entry = 26
        directory[entry : entry + 2] = (6).to_bytes(2, "little")
        directory[entry + 2 : entry + 4] = (7).to_bytes(2, "little")
        directory[entry + 4 : entry + 6] = (5).to_bytes(2, "little")
        directory[entry + 6] = 4
        directory[entry + 7 : entry + 11] = b"DATA"
        directory[entry + 0x16 : entry + 0x18] = (128).to_bytes(2, "little")
        for block in range(2, 6):
            block_data = directory[(block - 2) * BLOCK_SIZE : (block - 1) * BLOCK_SIZE]
            track, block_in_track = divmod(block, 8)
            for half in range(2):
                logical_sector = block_in_track * 2 + half
                physical_sector = order[logical_sector]
                raw = (track * 16 + physical_sector) * 256
                data[raw : raw + 256] = block_data[half * 256 : (half + 1) * 256]
        return bytes(data)

    def test_standard_pascal_directory_matches_2_ascending_view(self):
        disk = self.make_pascal_disk()
        valid, result = parse_pascal_directory(
            disk, SECTOR_ORDER_CANDIDATES["apple-pascal-2-ascending"]
        )
        self.assertTrue(valid)
        self.assertEqual(result["header"]["nextBlock"], 6)
        self.assertEqual(result["entries"][0]["name"], "DATA")

    def test_read_block_preserves_512_byte_logical_block(self):
        disk = self.make_pascal_disk()
        block = read_block(
            disk, 2, SECTOR_ORDER_CANDIDATES["apple-pascal-2-ascending"]
        )
        self.assertEqual(len(block), BLOCK_SIZE)
        self.assertEqual(block[7:11], b"TEST")

    def test_probe_keeps_source_hash_and_all_views(self):
        record = probe_disk("synthetic.dsk", self.make_pascal_disk())
        views = {item["view"]: item for item in record["directoryBlockProbe"]}
        self.assertTrue(views["apple-pascal-2-ascending"]["validStandardPascalDirectory"])
        self.assertEqual(set(views), set(SECTOR_ORDER_CANDIDATES))
        self.assertEqual(len(record["sha256"]), 64)


if __name__ == "__main__":
    unittest.main()
