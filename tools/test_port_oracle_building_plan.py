#!/usr/bin/env python3
"""port_oracle_building_plan 的判準測試：合成記憶體，不需要實跑或磁碟。"""

import io
import pathlib
import tempfile
import unittest
from contextlib import redirect_stdout

from check_city_map_runtime import COLUMNS, FIRST_COLUMN, FIRST_ROW, ROWS
from port_oracle_building_plan import (
    DOOR_OFFSETS,
    FACILITY_NAMES,
    STRIDE_X_BASE,
    VIEWPORT,
    cell_of,
    main,
    word,
)

SIZE = 0x20000
GEC = 0x1000
SCROLL_X_PTR, SCROLL_Y_PTR = 0x2000, 0x2002
BUFFER_BASE = 0x8000
STRIDE = 40  # 城市地面：0x20 + Lf，Lf = 8


def put_word(memory: bytearray, addr: int, value: int) -> None:
    """p-system 的 word 是 big-endian 存放的。"""
    memory[addr:addr + 2] = value.to_bytes(2, "big")


def build_memory(buildings, walker_cell=(7, 11), scroll=(7, 4), stride_common=8):
    """做一份只含這支工具需要的東西的記憶體：緩衝、Gec[0]、兩個捲動偏移。"""
    memory = bytearray(SIZE)
    stride_x = STRIDE_X_BASE + stride_common
    stride_y = 32 + stride_common
    put_word(memory, SCROLL_X_PTR, scroll[0])
    put_word(memory, SCROLL_Y_PTR, scroll[1])
    # 走路人站在 walker_cell 的中央。
    put_word(memory, GEC + 2, (walker_cell[1] - scroll[0]) * stride_x + stride_x // 2)
    put_word(memory, GEC + 4, (walker_cell[0] - scroll[1]) * stride_y + stride_y // 2)
    # 城市矩形內填一般地面，矩形外留 0——locate_buffer 靠這個形狀認出緩衝。
    for row in range(FIRST_ROW, FIRST_ROW + 12):
        for column in range(FIRST_COLUMN, FIRST_COLUMN + 20):
            memory[BUFFER_BASE + row * COLUMNS + column] = 1
    for code, row, column in buildings:
        memory[BUFFER_BASE + row * COLUMNS + column] = code
    return memory


def run(memory, **extra):
    with tempfile.TemporaryDirectory() as d:
        path = pathlib.Path(d) / "mem.bin"
        path.write_bytes(bytes(memory))
        argv = [
            "--memory", str(path), "--gec", hex(GEC),
            "--scroll-x-ptr", hex(SCROLL_X_PTR), "--scroll-y-ptr", hex(SCROLL_Y_PTR),
            "--stride-common", "8", "--stride-y-extra", "32",
            "--row", "7", "--column", "11",
        ]
        for key, value in extra.items():
            flag = f"--{key.replace('_', '-')}"
            argv += [flag] if value is None else [flag, str(value)]
        buffer = io.StringIO()
        with redirect_stdout(buffer):
            main(argv)
        return buffer.getvalue()


def settings(output: str) -> dict:
    line = next(l for l in output.splitlines() if l.startswith("SET "))
    return dict(pair.split("=", 1) for pair in line[4:].split())


class ConversionTest(unittest.TestCase):
    def test_the_round_trip_lands_on_the_cell_it_was_built_from(self):
        memory = build_memory([(35, 6, 13)], walker_cell=(9, 17))
        self.assertEqual(cell_of(memory, GEC, 7, 4, STRIDE, STRIDE), (9, 17))

    def test_words_are_read_big_endian(self):
        memory = bytearray(16)
        memory[0:2] = b"\x00\xbd"
        self.assertEqual(word(memory, 0), 0xBD)


class PositiveControlTest(unittest.TestCase):
    def test_a_mismatch_between_memory_and_the_debugger_stops_the_plan(self):
        # 走路人其實在 (9,17)，但呼叫端說除錯器讀到 (7,11)——資料不同時，不能繼續注入。
        memory = build_memory([(35, 6, 13)], walker_cell=(9, 17))
        with self.assertRaises(SystemExit):
            run(memory)


class DoorTest(unittest.TestCase):
    def test_the_target_is_the_door_not_the_middle_of_the_cell(self):
        memory = build_memory([(35, 6, 13)])
        got = settings(run(memory))
        door_x, door_y = DOOR_OFFSETS[19][0]
        self.assertEqual(int(got["TARGET_W1"]), (13 - 7) * STRIDE + door_x)
        self.assertEqual(int(got["TARGET_W2"]), (6 - 4) * STRIDE + door_y)
        # 站在格中央就會落在門的 ±4 之外，check_building 因此判不到門。
        self.assertNotEqual(int(got["TARGET_W1"]) % STRIDE, STRIDE // 2)

    def test_the_click_point_is_the_door_scaled_to_the_window(self):
        memory = build_memory([(35, 6, 13)])
        got = settings(run(memory))
        self.assertEqual(int(got["CLICK_X"]), int(got["TARGET_W1"]) * 4)
        self.assertEqual(int(got["CLICK_Y"]), int(got["TARGET_W2"]) * 4 + 80)


class ReachabilityTest(unittest.TestCase):
    def test_a_building_outside_the_viewport_is_not_picked(self):
        # 欄 23 的門落在 sprite x=654，遠在 320 寬的視埠之外，點不到。
        memory = build_memory([(34, 9, 23), (35, 6, 13)])
        got = settings(run(memory))
        self.assertEqual(int(got["TARGET_SLOT"]), 1)
        self.assertLess(int(got["TARGET_W1"]), VIEWPORT[0])

    def test_asking_for_an_unreachable_slot_fails_rather_than_silently_switching(self):
        memory = build_memory([(34, 9, 23), (35, 6, 13)])
        with self.assertRaises(SystemExit):
            run(memory, slot=0)

    def test_a_slot_whose_door_needs_an_extra_gate_is_deprioritised(self):
        # 兩個都在視埠內時，碼 18（另有 G17 閘門）要排在碼 19 後面。
        memory = build_memory([(34, 7, 12), (35, 6, 13)])
        got = settings(run(memory))
        self.assertEqual(int(got["TARGET_CODE"]), 19)

    def test_no_reachable_door_fails(self):
        memory = build_memory([(34, 9, 23)])
        with self.assertRaises(SystemExit):
            run(memory)


class ScrollTest(unittest.TestCase):
    """--scroll：視埠外的建築改用捲動偏移帶過去，而不是用走的。"""

    OUTSIDE = [(36, 7, 25)]  # 碼 20，門在 sprite x=744，遠在 320 寬的視埠外

    def test_without_scroll_an_outside_building_still_fails(self):
        with self.assertRaises(SystemExit):
            run(build_memory(self.OUTSIDE), slot=2)

    def test_scroll_brings_the_door_into_the_viewport(self):
        got = settings(run(build_memory(self.OUTSIDE), slot=2, scroll=None))
        self.assertLess(int(got["TARGET_W1"]), VIEWPORT[0])
        self.assertLess(int(got["TARGET_W2"]), VIEWPORT[1])
        self.assertEqual(int(got["TARGET_SLOT"]), 2)

    def test_the_new_scroll_and_sprite_still_resolve_to_the_target_cell(self):
        # 捲動偏移與 sprite 座標是同一條換算的兩端。只寫其中一個，check_exit 會算出
        # 第三個格子——所以這裡把兩個新值一起代回去驗。
        got = settings(run(build_memory(self.OUTSIDE), slot=2, scroll=None))
        scroll_x, scroll_y = int(got["TARGET_SCROLL_X"]), int(got["TARGET_SCROLL_Y"])
        w1, w2 = int(got["TARGET_W1"]), int(got["TARGET_W2"])
        self.assertEqual((scroll_y + w2 // STRIDE) % ROWS, int(got["TARGET_ROW"]))
        self.assertEqual((scroll_x + w1 // STRIDE) % COLUMNS, int(got["TARGET_COL"]))

    def test_the_door_offset_inside_the_cell_survives_the_scroll(self):
        # 捲過去之後仍然要站在門上，不是格中央——check_building 判的是格內偏移。
        got = settings(run(build_memory(self.OUTSIDE), slot=2, scroll=None))
        door = DOOR_OFFSETS[20][0]
        self.assertEqual((int(got["TARGET_W1"]) % STRIDE, int(got["TARGET_W2"]) % STRIDE), door)


if __name__ == "__main__":
    unittest.main()


class FacilityScanTest(unittest.TestCase):
    """設施掃描不能只看建築槽位：Uniteller 的碼來自城市標頭，值是 32 不是 34–36。"""

    def test_a_uniteller_cell_is_found(self):
        memory = build_memory([(32, 7, 13)])  # 地形碼 15 + 變體 16
        got = settings(run(memory))
        self.assertEqual(int(got["TARGET_CODE"]), 16)
        self.assertEqual(got["TARGET_FACILITY"], "uniteller")

    def test_scanning_by_value_threshold_alone_would_miss_it(self):
        # 這條釘住修好的那個 bug：舊掃描的判準是「值 >= 34」，32 會被漏掉。
        from port_oracle_building_plan import FIRST_BUILDING_CODE
        self.assertLess(32, FIRST_BUILDING_CODE)
        self.assertIn(16, FACILITY_NAMES)

    def test_a_facility_can_be_picked_by_name(self):
        memory = build_memory([(32, 7, 13), (35, 6, 13)])
        got = settings(run(memory, facility="uniteller"))
        self.assertEqual(got["TARGET_FACILITY"], "uniteller")
        got = settings(run(memory, facility="depot"))
        self.assertEqual(got["TARGET_FACILITY"], "depot")

    def test_one_city_may_hold_several_unitellers(self):
        # 碼 16 來自標頭欄位，同一座城市可以有好幾個——實測 Drahew 有三個。
        memory = build_memory([(32, 7, 13), (32, 9, 14), (32, 12, 19)])
        got = settings(run(memory, facility="uniteller"))
        self.assertEqual(got["TARGET_FACILITY"], "uniteller")

    def test_the_uniteller_door_offset_is_used(self):
        memory = build_memory([(32, 7, 13)])
        got = settings(run(memory))
        door = DOOR_OFFSETS[16][0]
        self.assertEqual((int(got["TARGET_W1"]) % STRIDE, int(got["TARGET_W2"]) % STRIDE), door)
