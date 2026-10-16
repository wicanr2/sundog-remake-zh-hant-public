#!/usr/bin/env python3
"""check_hatari_title_parity 的判準測試：用合成圖，不需要原版磁碟或模擬器。"""

import unittest

from PIL import Image

from check_title_pixel_parity import (
    DEFAULT_REGION,
    DYNAMIC_REGIONS,
    SOURCES,
    ST_SIZE,
    compare,
    quantize_decoder,
    quantize_hatari,
    st_frame,
)

ST_ORIGIN = SOURCES["hatari"]["origin"]
ST_SCALE = SOURCES["hatari"]["scale"]

# 兩支工具對同一個 3 位元色階的 8 位元寫法。
HATARI_LEVEL = [v * 2 * 17 for v in range(8)]
DECODER_LEVEL = [round(v * 255 / 7) for v in range(8)]


def make_reference(pattern) -> Image.Image:
    """用解碼工具的色階畫一張 320×200 參考圖。pattern(x,y) 回傳 3 位元 RGB。"""
    im = Image.new("RGB", ST_SIZE)
    px = im.load()
    for y in range(ST_SIZE[1]):
        for x in range(ST_SIZE[0]):
            px[x, y] = tuple(DECODER_LEVEL[v] for v in pattern(x, y))
    return im


def make_shot(pattern) -> Image.Image:
    """用 Hatari 的色階畫一張 832×552 截圖，ST 畫面放在約定位置並 2 倍放大。"""
    shot = Image.new("RGB", (832, 552), (200, 200, 200))
    frame = Image.new("RGB", ST_SIZE)
    px = frame.load()
    for y in range(ST_SIZE[1]):
        for x in range(ST_SIZE[0]):
            px[x, y] = tuple(HATARI_LEVEL[v] for v in pattern(x, y))
    shot.paste(
        frame.resize((ST_SIZE[0] * ST_SCALE, ST_SIZE[1] * ST_SCALE), Image.NEAREST),
        ST_ORIGIN,
    )
    return shot


def stripes(x, y):
    return ((x + y) % 8, x % 8, y % 8)


class QuantizeTest(unittest.TestCase):
    def test_both_scales_collapse_to_the_same_three_bit_value(self):
        for v in range(8):
            h = HATARI_LEVEL[v]
            d = DECODER_LEVEL[v]
            self.assertEqual(quantize_hatari((h, h, h)), (v, v, v))
            self.assertEqual(quantize_decoder((d, d, d)), (v, v, v))

    def test_the_two_scales_really_do_differ_in_eight_bit_form(self):
        # 若兩邊寫法相同，這支工具就沒有存在的必要；釘住它們確實不同。
        self.assertNotEqual(HATARI_LEVEL[5], DECODER_LEVEL[5])
        self.assertEqual((HATARI_LEVEL[5], DECODER_LEVEL[5]), (170, 182))


class FrameTest(unittest.TestCase):
    def test_st_frame_recovers_the_original_320x200(self):
        shot = make_shot(stripes)
        frame = st_frame(shot)
        self.assertEqual(frame.size, ST_SIZE)
        px = frame.load()
        self.assertEqual(px[10, 20], tuple(HATARI_LEVEL[v] for v in stripes(10, 20)))

    def test_a_shot_too_small_for_the_st_area_is_rejected(self):
        with self.assertRaises(ValueError):
            st_frame(Image.new("RGB", (320, 200)))


def make_port_shot(pattern) -> Image.Image:
    """port 的視窗：320×200 的 4 倍、沒有邊框，色階與解碼工具相同。"""
    frame = Image.new("RGB", ST_SIZE)
    px = frame.load()
    for y in range(ST_SIZE[1]):
        for x in range(ST_SIZE[0]):
            px[x, y] = tuple(DECODER_LEVEL[v] for v in pattern(x, y))
    return frame.resize((ST_SIZE[0] * 4, ST_SIZE[1] * 4), Image.NEAREST)


class PortSourceTest(unittest.TestCase):
    def test_port_geometry_and_scale_match_the_decoder(self):
        same, total, diffs = compare(
            make_port_shot(stripes), make_reference(stripes), DEFAULT_REGION, "port"
        )
        self.assertEqual(same, total)
        self.assertEqual(diffs, [])

    def test_reading_a_port_shot_as_hatari_geometry_is_rejected(self):
        # 1280×800 放得下 hatari 的框，但取到的是別的位置——所以判準是「比出來不對」，
        # 不是「尺寸不合」。這條釘住 --source 不能亂填。
        same, total, _ = compare(
            make_port_shot(stripes), make_reference(stripes), DEFAULT_REGION, "hatari"
        )
        self.assertLess(same, total)


class CompareTest(unittest.TestCase):
    def test_same_picture_in_two_colour_scales_is_a_full_match(self):
        same, total, diffs = compare(
            make_shot(stripes), make_reference(stripes), DEFAULT_REGION
        )
        self.assertEqual(same, total)
        self.assertEqual(diffs, [])

    def test_one_changed_pixel_inside_the_region_fails(self):
        ref = make_reference(stripes)
        ref.load()[10, 100] = tuple(DECODER_LEVEL[(v + 1) % 8] for v in stripes(10, 100))
        same, total, diffs = compare(make_shot(stripes), ref, DEFAULT_REGION)
        self.assertEqual(total - same, 1)
        self.assertEqual(diffs[0][:2], (10, 100))

    def test_a_change_outside_the_region_is_ignored(self):
        # 標誌與對話框蓋住的區域本來就無從比對，改那裡不該讓閘門變紅。
        ref = make_reference(stripes)
        ref.load()[200, 10] = tuple(DECODER_LEVEL[(v + 1) % 8] for v in stripes(200, 10))
        same, total, _ = compare(make_shot(stripes), ref, DEFAULT_REGION)
        self.assertEqual(same, total)

    def test_reference_of_the_wrong_size_is_rejected(self):
        with self.assertRaises(ValueError):
            compare(make_shot(stripes), Image.new("RGB", (160, 100)), DEFAULT_REGION)


class ExcludeTest(unittest.TestCase):
    def test_excluded_rectangles_drop_out_of_both_counts(self):
        same, total, _ = compare(
            make_shot(stripes), make_reference(stripes), (0, 0, 10, 10), "hatari",
            exclude=((0, 0, 4, 5),),
        )
        self.assertEqual((same, total), (80, 80))  # 100 − 4×5

    def test_a_difference_inside_an_excluded_rectangle_is_ignored(self):
        ref = make_reference(stripes)
        ref.load()[2, 2] = tuple(DECODER_LEVEL[(v + 1) % 8] for v in stripes(2, 2))
        same, total, _ = compare(
            make_shot(stripes), ref, (0, 0, 10, 10), "hatari", exclude=((0, 0, 4, 5),)
        )
        self.assertEqual(same, total)

    def test_the_cockpit_mask_is_the_documented_13443_static_pixels(self):
        # 這個數字是 compare_appimage_to_original 報告裡的 staticPixelsCompared；
        # 兩邊共用同一份矩形，這條測試釘住它們不會各自漂走。
        covered = sum(
            (x1 - x0) * (y1 - y0) for x0, y0, x1, y1 in DYNAMIC_REGIONS["cockpit"]
        )
        self.assertEqual(320 * 200 - covered, 13443)


if __name__ == "__main__":
    unittest.main()
