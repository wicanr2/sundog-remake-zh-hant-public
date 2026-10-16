#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的影像目錄。

輸入是唯讀的外層 Atari ST archive；工具只讀 nested raw ``.st``，不改寫原始媒體，
也不把影像 bytes 寫進輸出。輸出是含 raw offset、來源雜湊與逐筆長度／尺寸的 JSON。

原版取一張影像只給一個索引（`MAINLIB:0x4d fetch_dpic(addr, idx)`）。索引怎麼變成
磁碟位置，寫在 `MAINLIB:0x4b fetch_epic` 裡：它拿 `MAINLIB_G43f + idx × 2 words` 當
目錄項，第 0 個 word 是**起點**、第 1 個 word 是**長度**，兩者都以 word 為單位。
那張目錄由 `XSTARTUP:0x01` 以 `moveblock(1, 0xaf, 0, &G43f, 1, 0x47, 1)` 從 logical
block 0x47 搬進來，長度 176 words = 88 筆。

**驗收條件是首尾相接**：每一筆的起點必須等於前一筆的起點加長度，而且每一張影像的
前四個 byte 都要是合理的 big-endian 寬高。目錄若讀錯位置或錯了 endianness，這兩項
會大量失敗。

**未命名的部分**保留原值：這份工具只說「第 idx 張影像在磁碟的哪裡、多長、宣告多大」，
不宣稱任何一張是什麼內容。城市地表圖塊是哪幾張、每一格地形碼畫成哪一塊，另見
``docs/atari-st/city-maps.md``。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import struct
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


BLOCK_SIZE = 512
LOGICAL_BASE_OFFSET = 0x04800

# 目錄住在 logical block 0x47，176 words = 88 筆 × 2 words。
DIRECTORY_BLOCK = 0x47
DIRECTORY_WORDS = 176
DIRECTORY_ENTRIES = DIRECTORY_WORDS // 2

# 影像串流從 logical block 0x48 起算；目錄的起點是相對於它的 word 位移。
IMAGE_BASE_BLOCK = 0x48
IMAGE_HEADER_BYTES = 4

# 影像宣告的寬高合理範圍。原版畫面是 320×200，最寬的圖塊條是 496 像素。
MAX_IMAGE_WIDTH = 512
MAX_IMAGE_HEIGHT = 256


class ImageDirectoryError(ValueError):
    """影像目錄沒有照著已解出的版面排列。"""


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            return choose_disk(nested)


def directory_offset() -> int:
    return LOGICAL_BASE_OFFSET + DIRECTORY_BLOCK * BLOCK_SIZE


def image_stream_offset() -> int:
    return LOGICAL_BASE_OFFSET + IMAGE_BASE_BLOCK * BLOCK_SIZE


def decode_directory(disk: bytes) -> list[dict[str, object]]:
    start = directory_offset()
    raw = disk[start:start + DIRECTORY_WORDS * 2]
    if len(raw) != DIRECTORY_WORDS * 2:
        raise ImageDirectoryError("影像目錄讀不滿 176 words")

    stream = image_stream_offset()
    images: list[dict[str, object]] = []
    expected_start = 0
    for index in range(DIRECTORY_ENTRIES):
        offset_words, length_words = struct.unpack_from(">HH", raw, index * 4)
        if length_words == 0:
            # 目錄的尾巴是未使用的槽位，一律全零。
            if offset_words != 0:
                raise ImageDirectoryError(f"第 {index} 筆長度為 0 但起點是 {offset_words}")
            continue
        if offset_words != expected_start:
            raise ImageDirectoryError(
                f"第 {index} 筆從 word {offset_words} 起算，前一筆結束於 word {expected_start}"
            )
        raw_offset = stream + offset_words * 2
        header = disk[raw_offset:raw_offset + IMAGE_HEADER_BYTES]
        if len(header) != IMAGE_HEADER_BYTES:
            raise ImageDirectoryError(f"第 {index} 筆的標頭超出磁碟")
        width, height = struct.unpack(">HH", header)
        if not 0 < width <= MAX_IMAGE_WIDTH or not 0 < height <= MAX_IMAGE_HEIGHT:
            raise ImageDirectoryError(f"第 {index} 筆宣告 {width}×{height}，不是合理的影像尺寸")
        images.append({
            "index": index,
            "offsetWords": offset_words,
            "lengthWords": length_words,
            "rawOffset": f"0x{raw_offset:05x}",
            "byteLength": length_words * 2,
            "declaredWidth": width,
            "declaredHeight": height,
        })
        expected_start = offset_words + length_words
    if not images:
        raise ImageDirectoryError("影像目錄是空的")
    return images


def tile_strips(images: list[dict[str, object]]) -> list[dict[str, object]]:
    """32 像素高、寬度是 32 倍數的影像是圖塊條。

    城市地表用的就是這種：`XMOVEONG:0x03 read_chars` 依地圖標頭的三個欄位各取一張。
    這裡只標出形狀，不宣稱哪一格地形碼對應哪一塊。
    """
    strips = []
    for image in images:
        height = int(image["declaredHeight"])
        width = int(image["declaredWidth"])
        if height != 32 or width % 32 != 0:
            continue
        strips.append({
            "index": image["index"],
            "rawOffset": image["rawOffset"],
            "tileCount": width // 32,
            "tileSize": 32,
        })
    return strips


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)
    images = decode_directory(disk)
    strips = tile_strips(images)
    return {
        "schema": 1,
        "tool": "tools/atari_st_image_directory.py",
        "evidenceLevel": "proven",
        "source": {
            "archive": archive_path.name,
            "archiveSha256": sha256(archive_path.read_bytes()),
            "variant": variant,
            "disk": disk_name,
            "diskSha256": sha256(disk),
        },
        "layout": {
            "directoryLogicalBlock": DIRECTORY_BLOCK,
            "directoryRawOffset": f"0x{directory_offset():05x}",
            "directoryEntries": DIRECTORY_ENTRIES,
            "imageBaseLogicalBlock": IMAGE_BASE_BLOCK,
            "imageStreamRawOffset": f"0x{image_stream_offset():05x}",
            "entryFields": "big-endian word 0 = 起點（word），word 1 = 長度（word）",
        },
        "summary": {
            "imageCount": len(images),
            "contiguous": True,
            "totalBytes": sum(int(image["byteLength"]) for image in images),
            "tileStripCount": len(strips),
        },
        "tileStrips": strips,
        "images": images,
        "unresolved": [
            "每一張影像的內容與調色盤。",
            "地形碼怎麼對到圖塊已經解出——見 docs/atari-st/ship-rules.json 的 "
            "city.terrain-code、city.tile-sets、city.tile-pick、city.tile-threshold。"
            "沒解的是每一塊圖案畫出來是什麼地物（草地、水、路），那要看圖塊本身。",
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument("--output", type=Path, help="輸出 JSON 路徑；未指定則印到 stdout")
    args = parser.parse_args()

    document = build_document(args.archive, args.variant)
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
