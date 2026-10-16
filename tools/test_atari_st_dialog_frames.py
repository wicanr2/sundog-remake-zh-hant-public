#!/usr/bin/env python3
"""不依賴原始媒體的對話 frame 解碼器單元測試。

所有測試都在合成的 512-byte block 上跑，不讀取任何原版磁碟。合成器本身就是一份
規格：它照著解碼器宣稱的版面把偏移表、frame 記錄與 token 區接起來，測試再逐項把
某一段弄壞，確認解碼器擋得住而不是默默給出錯的答案。
"""

from __future__ import annotations

import unittest

from atari_st_dialog_frames import (
    BLOCK_SIZE,
    FRAME_FIRST_BLOCK,
    LOCAL_TOKEN_COUNT_BYTE,
    LOCAL_TOKEN_START_BYTE,
    LOCAL_TOKEN_START_SCALE,
    LOGICAL_BASE_OFFSET,
    OFFSET_TABLE_SIZE,
    FrameFormatError,
    block_offset,
    parse_block,
    parse_frame,
    store_stock,
)


def build_block(records: list[bytes], token_area: bytes = b"") -> bytes:
    """照解碼器宣稱的版面組出一個 block。"""
    block = bytearray(BLOCK_SIZE)
    cursor = OFFSET_TABLE_SIZE + 2
    for index, record in enumerate(records):
        block[index] = cursor
        block[cursor:cursor + len(record)] = record
        cursor += len(record)
    block[LOCAL_TOKEN_COUNT_BYTE] = 1 if token_area else 0
    # token 區的起點要是偶數，因為 block 只存它的一半。
    token_start = cursor + (cursor % LOCAL_TOKEN_START_SCALE)
    block[LOCAL_TOKEN_START_BYTE] = token_start // LOCAL_TOKEN_START_SCALE
    block[token_start:token_start + len(token_area)] = token_area
    return bytes(block)


def header(flags: int, buttons: int, step: int = 0) -> int:
    return (flags << 4) | (buttons << 2) | step


class BlockAddressingTests(unittest.TestCase):
    def test_logical_block_maps_to_the_documented_file_offset(self) -> None:
        # 世界表在 logical block 8、raw 0x05800，是這個換算的獨立對照點。
        self.assertEqual(block_offset(8), 0x05800)
        self.assertEqual(block_offset(FRAME_FIRST_BLOCK), LOGICAL_BASE_OFFSET + 20 * BLOCK_SIZE)


class FrameRecordTests(unittest.TestCase):
    def test_header_byte_unpacks_into_three_fields(self) -> None:
        record = bytes([header(0, 0, 3), 0x00, 0x99, 0x01, 0x02])
        frame = parse_frame(record + bytes(16), 0)
        self.assertEqual(frame["flags"], 0)
        self.assertEqual(frame["buttons"], 0)
        self.assertEqual(frame["displayStep"], 3)
        self.assertEqual(frame["length"], 5)

    def test_plain_frame_reads_message_and_two_slots(self) -> None:
        record = bytes([header(0, 0), 0x00, 0x99, 0x11, 0x22])
        frame = parse_frame(record + bytes(16), 0)
        self.assertEqual(frame["kind"], "message")
        self.assertEqual(frame["message"], 0x99)
        self.assertEqual([slot["value"] for slot in frame["slots"]], [0x11, 0x22])
        self.assertEqual(frame["buttonList"], [])

    def test_buttons_take_two_bytes_each(self) -> None:
        record = bytes([header(0, 2), 0x00, 0x99, 0x11, 0x22, 0x01, 0x40, 0x02, 0x41])
        frame = parse_frame(record + bytes(16), 0)
        self.assertEqual(frame["buttonList"], [
            {"label": 0x01, "target": 0x40},
            {"label": 0x02, "target": 0x41},
        ])
        self.assertEqual(frame["length"], 9)

    def test_slot_alternate_costs_one_byte_when_the_condition_is_zero(self) -> None:
        record = bytes([header(0x1, 0), 0x00, 0x99, 0x11, 0x00, 0x22, 0x05, 0x33])
        frame = parse_frame(record + bytes(16), 0)
        self.assertEqual(frame["slots"][0]["alternate"], [0x00])
        self.assertEqual(frame["slots"][1]["alternate"], [0x05, 0x33])
        self.assertEqual(frame["length"], 8)

    def test_button_alternate_follows_its_own_flag_bit(self) -> None:
        without = parse_frame(bytes([header(0x1, 1), 0x00, 0x99, 0x11, 0x00, 0x22, 0x00,
                                     0x01, 0x40]) + bytes(16), 0)
        self.assertNotIn("alternate", without["buttonList"][0])
        with_alternate = parse_frame(bytes([header(0x2, 1), 0x00, 0x99, 0x11, 0x22,
                                            0x01, 0x40, 0x07, 0x41]) + bytes(16), 0)
        self.assertEqual(with_alternate["buttonList"][0]["alternate"], [0x07, 0x41])

    def test_chain_frame_has_a_target_instead_of_a_message(self) -> None:
        # 旗標 bit1 加上零個按鈕就是「直接接到另一個 frame」；接續編號同樣要跑 pick_one。
        record = bytes([header(0x2, 0), 0x00, 0x02, 0xb1, 0xb2])
        frame = parse_frame(record + bytes(16), 0)
        self.assertEqual(frame["kind"], "chain")
        self.assertEqual(frame["target"], 0x02)
        self.assertEqual(frame["choices"], [0xb1, 0xb2])
        self.assertEqual(frame["length"], 5)
        self.assertNotIn("message", frame)

    def test_low_message_numbers_consume_pick_one_candidates(self) -> None:
        # 訊息編號 3 會吃掉三個候選位元組；30 以上一個都不吃。
        picked = parse_frame(bytes([header(0, 0), 0x00, 3, 0xa1, 0xa2, 0xa3, 0x11, 0x22])
                             + bytes(16), 0)
        self.assertEqual(picked["choices"], [0xa1, 0xa2, 0xa3])
        self.assertEqual(picked["length"], 8)
        plain = parse_frame(bytes([header(0, 0), 0x00, 30, 0x11, 0x22]) + bytes(16), 0)
        self.assertEqual(plain["choices"], [])
        self.assertEqual(plain["length"], 5)

    def test_postcode_operand_count_follows_the_opcode(self) -> None:
        for opcode, operands in ((0, 0), (1, 1), (2, 2), (3, 4), (4, 0), (5, 1)):
            record = bytes([header(0x8, 0), 0x00, 0x99, 0x11, 0x22, opcode]) + bytes(range(operands))
            frame = parse_frame(record + bytes(16), 0)
            self.assertEqual(frame["postcode"]["opcode"], opcode)
            self.assertEqual(len(frame["postcode"]["operands"]), operands)
            self.assertEqual(frame["length"], 6 + operands)

    def test_a_frame_running_past_the_block_is_rejected(self) -> None:
        with self.assertRaises(FrameFormatError):
            parse_frame(bytes([header(0, 3), 0x00]), 0)


class BlockTilingTests(unittest.TestCase):
    def test_records_that_tile_exactly_are_accepted(self) -> None:
        records = [
            bytes([header(0, 0), 0x00, 0x99, 0x11, 0x22]),
            bytes([header(0, 1), 0x00, 0x98, 0x11, 0x22, 0x01, 0x40]),
        ]
        parsed = parse_block(build_block(records, b"\x03abc"), FRAME_FIRST_BLOCK)
        self.assertEqual([frame["length"] for frame in parsed["frames"]], [5, 7])
        self.assertEqual(parsed["frames"][0]["id"], OFFSET_TABLE_SIZE)
        self.assertEqual(parsed["frames"][1]["id"], OFFSET_TABLE_SIZE + 1)

    def test_a_record_that_ends_short_of_the_next_offset_is_rejected(self) -> None:
        block = bytearray(build_block([
            bytes([header(0, 0), 0x00, 0x99, 0x11, 0x22]),
            bytes([header(0, 0), 0x00, 0x99, 0x11, 0x22]),
        ]))
        # 把第二筆的起點往後推一格，第一筆就對不上了。
        block[1] += 1
        with self.assertRaises(FrameFormatError):
            parse_block(bytes(block), FRAME_FIRST_BLOCK)

    def test_non_zero_bytes_between_the_last_record_and_the_tokens_are_rejected(self) -> None:
        block = bytearray(build_block([bytes([header(0, 0), 0x00, 0x99, 0x11, 0x22])]))
        block[LOCAL_TOKEN_START_BYTE] += 2
        block[OFFSET_TABLE_SIZE + 2 + 5] = 0x7f
        with self.assertRaises(FrameFormatError):
            parse_block(bytes(block), FRAME_FIRST_BLOCK)

    def test_zero_padding_before_the_token_area_is_accepted(self) -> None:
        block = bytearray(build_block([bytes([header(0, 0), 0x00, 0x99, 0x11, 0x22])]))
        block[LOCAL_TOKEN_START_BYTE] += 2
        parsed = parse_block(bytes(block), FRAME_FIRST_BLOCK)
        self.assertEqual(len(parsed["frames"]), 1)


class StoreStockTests(unittest.TestCase):
    def test_a_rising_pair_is_a_plain_item_range(self) -> None:
        frames = [{"id": 16, "postcode": {"opcode": 3, "operands": [0, 0, 8, 10]}}]
        self.assertEqual(store_stock(frames), [
            {"frameId": 16, "firstItem": 8, "lastItemByte": 10, "items": [8, 9, 10]},
        ])

    def test_a_falling_pair_defers_to_the_runtime_table(self) -> None:
        frames = [{"id": 28, "postcode": {"opcode": 3, "operands": [0, 0, 15, 1]}}]
        entry = store_stock(frames)[0]
        self.assertTrue(entry["lastItemFromRuntimeTable"])
        self.assertNotIn("items", entry)

    def test_other_opcodes_are_not_stock(self) -> None:
        frames = [{"id": 16, "postcode": {"opcode": 2, "operands": [1, 2]}}, {"id": 17}]
        self.assertEqual(store_stock(frames), [])


if __name__ == "__main__":
    unittest.main()
