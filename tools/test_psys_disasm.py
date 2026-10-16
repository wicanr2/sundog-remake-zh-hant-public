#!/usr/bin/env python3
"""psys_disasm 的判準測試：合成位元組，不需要 dump。"""

import struct
import unittest

from psys_disasm import decode_one, jump_table, segment_info


def decode(hexstr):
    return decode_one(bytes.fromhex(hexstr), 0)


class OperandTest(unittest.TestCase):
    def test_an_opcode_without_operands_is_one_byte(self):
        self.assertEqual(decode("00")["name"], "sldc0")
        self.assertEqual(decode("00")["length"], 1)

    def test_the_big_encoding_takes_one_byte_below_0x80(self):
        # lae 0x4, 0x2a —— 第二個運算元小於 0x80，一個位元組就夠。
        instruction = decode("9b 04 2a".replace(" ", ""))
        self.assertEqual([a["value"] for a in instruction["args"]], [4, 0x2a])
        self.assertEqual(instruction["length"], 3)

    def test_the_big_encoding_takes_two_bytes_at_or_above_0x80(self):
        # lae 0x4, 0x3a1 —— 0x83 0xa1 是 ((0x83 & 0x7f) << 8) | 0xa1。
        instruction = decode("9b0483a1")
        self.assertEqual([a["value"] for a in instruction["args"]], [4, 0x3a1])
        self.assertEqual(instruction["length"], 4)

    def test_a_word_operand_is_little_endian_regardless_of_segment_endian(self):
        # ldci 0x1e0 —— 指令流裡的 word 永遠 LE，與段的 endian 旗標無關。
        instruction = decode("81e001")
        self.assertEqual(instruction["args"][0]["value"], 0x1e0)

    def test_a_relative_jump_resolves_against_the_end_of_the_instruction(self):
        # ujp +2：指令佔 2 bytes，所以目標是 0 + 2 + 2 = 4。
        instruction = decode("8a02")
        self.assertEqual(instruction["args"][0]["target"], 4)
        self.assertTrue(instruction["args"][0]["relative"])

    def test_an_unknown_opcode_does_not_crash_the_decoder(self):
        # 資料被當成程式碼解時一定會撞到未定義的編號；反組譯器要能繼續往下走。
        data = bytes([0x40])
        self.assertEqual(decode_one(data, 0)["length"], 1)


def make_segment(*, name=b"TESTSEG", endian, cpool_words, tail=b""):
    header = bytearray(0x18)
    header[0x04:0x04 + len(name)] = name
    struct.pack_into("<H", header, 0x0c, endian)
    fmt = ">H" if endian != 0x0001 else "<H"
    struct.pack_into(fmt, header, 0x0e, cpool_words)
    return bytes(header) + tail


class SegmentTest(unittest.TestCase):
    def test_a_native_endian_segment_reads_its_own_fields_little_endian(self):
        info = segment_info(make_segment(endian=0x0001, cpool_words=0x1234), 0)
        self.assertEqual(info["name"], "TESTSEG")
        self.assertFalse(info["endianFlip"])
        self.assertEqual(info["cpoolWords"], 0x1234)

    def test_a_flipped_segment_reads_its_fields_big_endian(self):
        info = segment_info(make_segment(endian=0x0100, cpool_words=0x1473), 0)
        self.assertTrue(info["endianFlip"])
        self.assertEqual(info["cpoolWords"], 0x1473)


class JumpTableTest(unittest.TestCase):
    def test_the_table_bounds_and_deltas_come_back_verbatim(self):
        # 常數池要挑得夠遠，跳表才不會落進段標頭裡。
        cpool_words, operand = 0x10, 3
        memory = bytearray(make_segment(endian=0x0100, cpool_words=cpool_words))
        memory += bytes(64)
        table_at = (cpool_words + operand) * 2
        struct.pack_into(">h", memory, table_at, 10)
        struct.pack_into(">h", memory, table_at + 2, 12)
        for index, delta in enumerate((-190, 0, +5)):
            struct.pack_into(">h", memory, table_at + 4 + index * 2, delta)
        info = segment_info(bytes(memory), 0)
        table = jump_table(bytes(memory), 0, info, operand)
        self.assertEqual((table["min"], table["max"]), (10, 12))
        self.assertEqual(table["deltas"], [-190, 0, 5])
        self.assertEqual(table["tableOffset"], table_at)

    def test_the_table_is_read_little_endian_for_a_native_segment(self):
        cpool_words, operand = 0x10, 1
        memory = bytearray(make_segment(endian=0x0001, cpool_words=cpool_words))
        memory += bytes(64)
        table_at = (cpool_words + operand) * 2
        struct.pack_into("<h", memory, table_at, 4)
        struct.pack_into("<h", memory, table_at + 2, 5)
        struct.pack_into("<h", memory, table_at + 4, -7)
        struct.pack_into("<h", memory, table_at + 6, 9)
        info = segment_info(bytes(memory), 0)
        table = jump_table(bytes(memory), 0, info, operand)
        self.assertEqual((table["min"], table["max"]), (4, 5))
        self.assertEqual(table["deltas"], [-7, 9])


if __name__ == "__main__":
    unittest.main()
