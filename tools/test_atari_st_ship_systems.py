#!/usr/bin/env python3
"""不依賴原始媒體的船艦系統呼叫點解碼器單元測試。

所有測試都用合成的位元組流，不讀取任何原版磁碟。
"""

from __future__ import annotations

import unittest

from atari_st_ship_systems import (
    CALL_OPCODE,
    CALL_PROCEDURE,
    build_systems,
    decode_call_sites,
    find_constant_call_sites,
)


ITEM_NAMES = {
    0x13: "concentrator",
    0x14: "decloaker",
    0x15: "cloaker",
    0x16: "ground scanner",
    0x17: "shunt",
    0x18: "cryofuse",
    0x19: "j-junc module",
    0x1A: "photon bridge",
    0x1B: "scanner",
    0x1C: "flux modulator",
    0x1D: "plasma tube",
    0x1E: "s/t distorter",
    0x1F: "control node",
}

# 六個呼叫的參數，形狀與原版相同：系統 0 的 index 由全域值供給。
CALLS = [
    (0x1C, 0x1A, 0x1E, 0x00, 0x00),
    (0x1C, 0x1E, 0x18, 0x00, 0x00),
    (0x18, 0x1A, 0x1D, 0x13, 0x02),
    (0x1B, 0x19, 0x1A, 0x14, 0x02),
    (0x18, 0x1C, 0x1C, 0x15, 0x01),
    (0x19, 0x1B, 0x19, 0x16, 0x02),
]

# lde 0x4,0xe1：系統 0 的 index 來源，不是 sldc。
GLOBAL_PUSH = bytes([0x9A, 0x04, 0x80, 0xE1])
CALL = bytes([CALL_OPCODE, CALL_PROCEDURE])


def synthetic_stream(calls=CALLS, *, prefix: bytes = b"\xff\xff") -> bytes:
    stream = bytearray(prefix)
    stream += GLOBAL_PUSH + bytes(calls[0]) + CALL
    for index, params in enumerate(calls[1:], start=1):
        stream += bytes([index]) + bytes(params) + CALL
    stream += b"\xff\xff"
    return bytes(stream)


class FindCallSiteTests(unittest.TestCase):
    def test_finds_only_the_constant_parameter_calls(self) -> None:
        sites = find_constant_call_sites(synthetic_stream())
        self.assertEqual(len(sites), 5)

    def test_ignores_a_call_whose_parameters_are_not_constants(self) -> None:
        # 參數裡出現 >= 0x20 的 byte，就不是六個 sldc 的呼叫點。
        stream = bytes([0x01, 0x99, 0x1E, 0x18, 0x00, 0x00]) + CALL
        self.assertEqual(find_constant_call_sites(stream), [])


class DecodeCallSiteTests(unittest.TestCase):
    def test_decodes_six_systems_with_index_zero_by_elimination(self) -> None:
        calls = decode_call_sites(synthetic_stream())
        self.assertEqual([call["params"][0] for call in calls], [0, 1, 2, 3, 4, 5])
        self.assertTrue(calls[0]["indexFromGlobal"])
        self.assertEqual(calls[2]["params"], [2, 0x18, 0x1A, 0x1D, 0x13, 0x02])

    def test_rejects_a_stream_missing_a_call(self) -> None:
        with self.assertRaises(ValueError):
            decode_call_sites(synthetic_stream(CALLS[:5]))

    def test_rejects_non_adjacent_call_sites(self) -> None:
        stream = bytearray(synthetic_stream())
        # 在第二與第三個呼叫之間塞一個 byte，破壞相鄰性。
        insert_at = stream.index(CALL) + len(CALL) + 8
        stream[insert_at:insert_at] = b"\x00"
        with self.assertRaises(ValueError):
            decode_call_sites(bytes(stream))

    def test_rejects_out_of_order_system_indices(self) -> None:
        shuffled = [CALLS[0], CALLS[2], CALLS[1], CALLS[3], CALLS[4], CALLS[5]]
        stream = bytearray(b"\xff\xff")
        stream += GLOBAL_PUSH + bytes(shuffled[0]) + CALL
        for index, params in zip([2, 1, 3, 4, 5], shuffled[1:]):
            stream += bytes([index]) + bytes(params) + CALL
        with self.assertRaises(ValueError):
            decode_call_sites(bytes(stream))


class BuildSystemTests(unittest.TestCase):
    def test_column_zero_is_the_control_node_and_is_not_a_parameter(self) -> None:
        systems = build_systems(decode_call_sites(synthetic_stream()), ITEM_NAMES)
        for system in systems:
            self.assertEqual(system["columns"][0]["itemName"], "control node")
            self.assertEqual(len(system["columns"]), 4)

    def test_alternate_part_is_recorded_only_when_present(self) -> None:
        systems = build_systems(decode_call_sites(synthetic_stream()), ITEM_NAMES)
        self.assertIsNone(systems[0]["alternate"])
        self.assertEqual(systems[2]["alternate"]["itemName"], "concentrator")
        self.assertEqual(systems[2]["alternate"]["column"], 2)

    def test_rejects_a_part_outside_the_item_range(self) -> None:
        calls = decode_call_sites(synthetic_stream())
        calls[0]["params"][1] = 0x09
        with self.assertRaises(ValueError):
            build_systems(calls, ITEM_NAMES)

    def test_rejects_a_part_absent_from_the_item_table(self) -> None:
        calls = decode_call_sites(synthetic_stream())
        with self.assertRaises(ValueError):
            build_systems(calls, {key: value for key, value in ITEM_NAMES.items() if key != 0x1C})

    def test_rejects_an_alternate_in_column_zero(self) -> None:
        # 第 0 欄是 control node，不接受替代零件。
        calls = decode_call_sites(synthetic_stream())
        calls[2]["params"][5] = 0
        calls[2]["params"][4] = 0x13
        with self.assertRaises(ValueError):
            build_systems(calls, ITEM_NAMES)


if __name__ == "__main__":
    unittest.main()
