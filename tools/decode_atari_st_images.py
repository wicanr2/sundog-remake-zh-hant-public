#!/usr/bin/env python3
"""解碼 Atari ST SunDog raw image stream 的研究工具。

輸入只讀取外層 Atari ST archive 的 nested raw .st；輸出必須明確指定到暫存或
使用者自己的研究目錄。工具不會把原始磁碟、壓縮 payload 或解出的原始資產寫入
repository，只輸出 PNG 預覽與含來源 hash／raw offset 的 manifest。

這個格式與公開 secondary oracle 的 ``util_img_decompress_image`` 對齊：影像標頭
為 big-endian width／height，像素是 4-bit index，壓縮指令會以 nibble bit-reverse
還原 Atari ST 的 pixel order。它只能證明「可解碼的 ST image stream」，不能單獨
證明某一格是 tileset、碰撞資料或遊戲中的語意 frame。
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


IMAGE_START = 0x0D800
IMAGE_BLOCK_SIZE = 512
IMAGE_BLOCK_COUNT = 260
IMAGE_END_EXCLUSIVE = IMAGE_START + IMAGE_BLOCK_SIZE * IMAGE_BLOCK_COUNT

# 這兩個跳過範圍來自 secondary oracle 的 ripper 控制流程。它們只說明 image
# stream walker 不把這些 bytes 視為一般 image record；內容的遊戲語意仍是 unknown。
EXPLICIT_STREAM_GAPS = {
    (0x150FA, 0x15170): "secondary-ripper-explicit-skip",
    (0x2BCE4, 0x2BD14): "secondary-ripper-explicit-skip",
}

# 公開 laanwj/sundog 的 doc/disklayout.md 所列 image stream 邊界。名稱只是
# secondary oracle 的位置註記，consumer／anchor 仍是 unknown。
KNOWN_IMAGE_LABELS = {
    0x0D800: "title-candidate",
    0x0FEB8: "logo-candidate",
    0x101E0: "screen-candidate-1",
    0x11552: "items-candidate",
    0x11B9C: "narrow-candidate",
    0x126CA: "ship-interior-candidate",
    0x13726: "screen-candidate-2",
    0x1B166: "background-candidate",
    0x1B35A: "special-buildings-candidate",
    0x1CD18: "buildings-candidate-1",
    0x1D3B6: "buildings-candidate-2",
}

# rip_images.c 內的 Atari ST 16-colour palette。它是畫面轉換用 palette，不能被
# 解讀成 Apple II composite artifact palette。
PALETTE = (
    (0x00, 0x00, 0x00, 0xFF),
    (0x00, 0x00, 0xB6, 0xFF),
    (0x00, 0x92, 0x00, 0xFF),
    (0x00, 0xB6, 0xB6, 0xFF),
    (0xB6, 0x00, 0x00, 0xFF),
    (0xB6, 0x00, 0xB6, 0xFF),
    (0x92, 0x6D, 0x00, 0xFF),
    (0xB6, 0xB6, 0xB6, 0xFF),
    (0x49, 0x49, 0x49, 0xFF),
    (0x24, 0x49, 0xFF, 0xFF),
    (0x00, 0xFF, 0x00, 0xFF),
    (0x00, 0xFF, 0xFF, 0xFF),
    (0xFF, 0x00, 0x00, 0xFF),
    (0xFF, 0x00, 0xFF, 0xFF),
    (0xFF, 0xFF, 0x00, 0xFF),
    (0xFF, 0xFF, 0xFF, 0xFF),
)


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def flip4(value: int) -> int:
    return ((value & 0x1) << 3) | ((value & 0x2) << 1) | ((value & 0x4) >> 1) | ((value & 0x8) >> 3)


def read_byte(source: bytes, cursor: int) -> tuple[int, int]:
    if cursor >= len(source):
        raise ValueError(f"compressed image ends while reading byte at 0x{cursor:x}")
    return source[cursor], cursor + 1


def append_pixels(pixels: bytearray, values: list[int], total: int) -> None:
    if len(pixels) + len(values) > total:
        raise ValueError(
            f"compressed image overshoots {total} pixels by {len(pixels) + len(values) - total}"
        )
    pixels.extend(values)


def decompress_image(source: bytes, width: int, height: int) -> tuple[bytes, int]:
    """回傳 one-byte-per-pixel index buffer 與消耗的 compressed bytes。"""

    if width <= 0 or height <= 0:
        raise ValueError(f"invalid image dimensions {width}x{height}")
    total = width * height
    pixels = bytearray()
    cursor = 0
    while len(pixels) < total:
        opcode, cursor = read_byte(source, cursor)
        if opcode & 0x80:
            count_byte, cursor = read_byte(source, cursor)
            count = count_byte
            if opcode & 0x40:
                low_byte, cursor = read_byte(source, cursor)
                count = (count << 8) | low_byte
            count += 1
            if opcode & 0x10:
                if opcode & 0x20:
                    # 先複製上一行，再寫入一個 literal pixel；這裡刻意照
                    # secondary oracle 的 byte-level 行為實作。
                    if len(pixels) < width:
                        raise ValueError("previous-line command appears before the first row")
                    for _ in range(count):
                        if len(pixels) >= total:
                            raise ValueError("previous-line command overshoots image height")
                        # 必須逐像素 append：當 count 大於一個 scanline 時，
                        # secondary oracle 會讀到剛複製出的新 pixel。
                        pixels.append(pixels[len(pixels) - width])
                    append_pixels(pixels, [flip4(opcode & 0x0F)], total)
                else:
                    if count & 1:
                        append_pixels(pixels, [flip4(opcode & 0x0F)], total)
                    for _ in range(count // 2):
                        packed, cursor = read_byte(source, cursor)
                        append_pixels(
                            pixels,
                            [flip4(packed >> 4), flip4(packed & 0x0F)],
                            total,
                        )
            else:
                append_pixels(pixels, [flip4(opcode & 0x0F)] * count, total)
        else:
            count = (opcode >> 4) + 1
            append_pixels(pixels, [flip4(opcode & 0x0F)] * count, total)
    return bytes(pixels), cursor


def png_chunk(kind: bytes, payload: bytes) -> bytes:
    return struct.pack(">I", len(payload)) + kind + payload + struct.pack(">I", zlib_crc32(kind + payload))


def zlib_crc32(payload: bytes) -> int:
    # Import lazily so the decoder and manifest path remain stdlib-only, while
    # still producing a normal PNG for visual review.
    import zlib

    return zlib.crc32(payload) & 0xFFFFFFFF


def write_png(path: Path, width: int, height: int, pixels: bytes) -> None:
    import zlib

    rows = bytearray()
    for row in range(height):
        rows.append(0)  # PNG filter type: None
        start = row * width
        for pixel in pixels[start : start + width]:
            rows.extend(PALETTE[pixel])
    raw = b"\x89PNG\r\n\x1a\n"
    raw += png_chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0))
    raw += png_chunk(b"IDAT", zlib.compress(bytes(rows), level=9))
    raw += png_chunk(b"IEND", b"")
    path.write_bytes(raw)


def decode_record(disk: bytes, address: int) -> tuple[dict[str, object], bytes]:
    if address < 0 or address + 4 > len(disk):
        raise ValueError(f"image header at 0x{address:x} is outside disk")
    width, height = struct.unpack_from(">HH", disk, address)
    pixels, compressed_size = decompress_image(disk[address + 4 :], width, height)
    end = address + 4 + compressed_size
    if end > len(disk):
        raise ValueError(f"image at 0x{address:x} ends at 0x{end:x}, beyond disk")
    aligned_end = end + (end & 1)
    if aligned_end > len(disk):
        aligned_end = end
    record_span = disk[address:aligned_end]
    compressed_payload = disk[address + 4 : end]
    return {
        "rawOffset": f"0x{address:x}",
        "rawOffsetDecimal": address,
        "width": width,
        "height": height,
        "compressedPayloadOffset": f"0x{address + 4:x}",
        "compressedPayloadEndExclusive": f"0x{end:x}",
        "compressedSize": compressed_size,
        "spanEndExclusive": f"0x{aligned_end:x}",
        "recordSpanSize": len(record_span),
        "recordSpanSha256": sha256(record_span),
        "compressedPayloadSha256": sha256(compressed_payload),
        "alignmentPaddingBytes": aligned_end - end,
        "pixelCount": len(pixels),
        "pixelSha256": sha256(pixels),
        "label": KNOWN_IMAGE_LABELS.get(address, "unlabelled-image-candidate"),
        "evidence": {
            "proven": "big-endian dimensions、壓縮 stream 邊界與 one-byte-per-pixel 解碼可重現",
            "strongInference": "此 raw span 是 Atari ST image stream，label 來自 secondary oracle offset table",
            "unknown": "runtime consumer、tileset／sprite 語意、anchor、collision 與 Apple II 對應",
        },
    }, pixels


def describe_image_stream_coverage(
    disk: bytes,
    records: list[tuple[dict[str, object], bytes]],
    *,
    start: int = IMAGE_START,
    end: int = IMAGE_END_EXCLUSIVE,
) -> dict[str, object]:
    """回傳 image region 的結構覆蓋率，不賦予未解 bytes 遊戲語意。"""

    if start < 0 or end < start or end > len(disk):
        raise ValueError(f"invalid image region 0x{start:x}..0x{end:x} for disk size {len(disk)}")

    gaps: list[dict[str, object]] = []
    covered_bytes = 0
    cursor = start

    def append_gap(gap_start: int, gap_end: int) -> None:
        raw = disk[gap_start:gap_end]
        item: dict[str, object] = {
            "rawOffset": f"0x{gap_start:x}",
            "spanEndExclusive": f"0x{gap_end:x}",
            "byteCount": gap_end - gap_start,
            "rawSpanSha256": sha256(raw),
            "allBytesZero": not any(raw),
            "kind": EXPLICIT_STREAM_GAPS.get((gap_start, gap_end), "unclassified-gap"),
            "evidence": {
                "proven": "raw span boundary、byte count、SHA-256 與 zero/non-zero 狀態",
                "strongInference": "walker 不將本 span 解作一般 image record",
                "unknown": "此 span 的遊戲資料／素材／控制語意",
            },
        }
        if item["kind"] == "unclassified-gap" and gap_start + 4 <= gap_end:
            width, height = struct.unpack_from(">HH", disk, gap_start)
            if width == 0 or height == 0:
                item["kind"] = "zero-dimension-termination-or-padding"
                item["zeroDimensionHeader"] = {"width": width, "height": height}
        gaps.append(item)

    for record, _pixels in records:
        record_start = int(record["rawOffsetDecimal"])
        record_end = int(str(record["spanEndExclusive"]), 0)
        if record_start < cursor:
            raise ValueError(
                f"image record 0x{record_start:x} overlaps previous span ending at 0x{cursor:x}"
            )
        if record_end > end:
            raise ValueError(f"image record ending at 0x{record_end:x} exceeds region end 0x{end:x}")
        if record_start > cursor:
            append_gap(cursor, record_start)
        covered_bytes += record_end - record_start
        cursor = record_end

    if cursor < end:
        append_gap(cursor, end)

    return {
        "rawOffset": f"0x{start:x}",
        "spanEndExclusive": f"0x{end:x}",
        "totalBytes": end - start,
        "decodedRecordCount": len(records),
        "decodedRecordSpanBytes": covered_bytes,
        "gapBytes": sum(int(item["byteCount"]) for item in gaps),
        "gaps": gaps,
        "evidence": {
            "proven": "record span 與 gap span 在指定 image region 內完整且不重疊地覆蓋 raw bytes",
            "strongInference": "secondary oracle 的 explicit skip 對應 image stream walker 邊界",
            "unknown": "未解 gap 或各 record 的遊戲內素材／畫面 consumer 語意",
        },
    }


def walk_images(disk: bytes, start: int) -> list[tuple[dict[str, object], bytes]]:
    records: list[tuple[dict[str, object], bytes]] = []
    address = start
    while address + 4 <= len(disk):
        width, height = struct.unpack_from(">HH", disk, address)
        if width == 0 or height == 0:
            break
        record, pixels = decode_record(disk, address)
        records.append((record, pixels))
        address = int(record["rawOffsetDecimal"]) + 4 + int(record["compressedSize"])
        address += address & 1
        # These are the explicit skips in the secondary oracle's ripper.
        for (gap_start, gap_end), _kind in EXPLICIT_STREAM_GAPS.items():
            if address == gap_start:
                address = gap_end
                break
    return records


def load_disk(archive_path: Path, variant: str) -> tuple[dict[str, object], bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        nested_name, nested_bytes = choose_nested(outer, variant)
    with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
        disk_name, disk = choose_disk(nested)
    source = {
        "archive": archive_path.name,
        "archiveSha256": sha256(archive_bytes),
        "nestedArchive": nested_name,
        "nestedArchiveSha256": sha256(nested_bytes),
        "disk": disk_name,
        "diskSha256": sha256(disk),
        "diskSize": len(disk),
    }
    return source, disk


def parse_address(value: str) -> int:
    return int(value, 0)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--variant", choices=("[a2]", "[a]"), default="[a2]")
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument(
        "--address",
        action="append",
        type=parse_address,
        help="只輸出指定 raw image offset；可重複指定，預設輸出 image stream 全部候選",
    )
    args = parser.parse_args()

    source, disk = load_disk(args.archive, args.variant)
    all_records = walk_images(disk, IMAGE_START)
    wanted = set(args.address) if args.address else None
    selected = [item for item in all_records if wanted is None or item[0]["rawOffsetDecimal"] in wanted]
    if wanted is not None and wanted != {item[0]["rawOffsetDecimal"] for item in selected}:
        missing = sorted(wanted - {item[0]["rawOffsetDecimal"] for item in selected})
        raise SystemExit("requested image offsets were not decoded: " + ", ".join(f"0x{x:x}" for x in missing))

    args.output_dir.mkdir(parents=True, exist_ok=True)
    manifest_records: list[dict[str, object]] = []
    for record, pixels in selected:
        address = int(record["rawOffsetDecimal"])
        filename = f"{record['label']}-{address:05x}-{record['width']}x{record['height']}.png"
        write_png(args.output_dir / filename, int(record["width"]), int(record["height"]), pixels)
        manifest_record = dict(record)
        manifest_record["output"] = filename
        manifest_records.append(manifest_record)

    manifest = {
        "tool": "tools/decode_atari_st_images.py",
        "source": source,
        "variant": args.variant,
        "startOffset": f"0x{IMAGE_START:x}",
        "imageRegion": describe_image_stream_coverage(disk, all_records),
        "decodedCount": len(manifest_records),
        "records": manifest_records,
        "interpretation": {
            "proven": [
                "來源 archive／nested archive／raw disk hash、大小與 member 名稱",
                "image raw offset、big-endian dimensions、compressed span 與 decoded pixel hash",
            ],
            "strongInference": [
                "raw span 使用公開 secondary oracle 的 4-bit image decompressor",
                "known label 只代表 offset table 的研究名稱",
            ],
            "unknown": [
                "tileset frame identity、anchor、collision、runtime consumer 與 Apple II 對應",
            ],
        },
    }
    (args.output_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
