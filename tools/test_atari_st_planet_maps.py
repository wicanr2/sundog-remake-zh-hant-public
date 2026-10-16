import unittest

from atari_st_planet_maps import decode_record


class PlanetMapTests(unittest.TestCase):
    def test_length_is_exact(self):
        for length in (0, 255, 257):
            with self.assertRaises(ValueError):
                decode_record(bytes(length))

    def test_word_bit_order_and_row_reset(self):
        record = bytearray(256)
        # 首 word 的符號為 0,1,2,3,0,1,2,3（大端 word，從最低位起）。
        record[16:18] = bytes.fromhex("e4e4")
        rows = decode_record(record)
        self.assertEqual(rows[0][:8], [6, 5, 9, 13, 2, 8, 9, 13])
        self.assertEqual(rows[0][8], 2)
        self.assertEqual(rows[1][0], 6)
        self.assertEqual((len(rows), len(rows[-1])), (24, 40))

    def test_last_cell_uses_last_word(self):
        record = bytearray(256)
        record[-2:] = bytes.fromhex("ffff")
        rows = decode_record(record)
        self.assertEqual(rows[-1][-8:], [13] * 8)
        self.assertEqual(rows[-2][-1], 1)
