#!/usr/bin/env python3
"""將 MAME 的受限 HGR writer code sample 與唯讀 Apple II DSK bytes 對照。

輸入 log 只應位於 `/tmp`，因為它含有原版 runtime RAM 的暫存 code window。本工具不會把
這些 bytes 寫入結果；結果只保留 sample 雜湊、候選 DSK view 及可回查的 raw segment。
精確 byte match 只證明兩端 bytes 相同；它不是 loader、函式名稱、資料格式或 remake RAM
依賴的證明。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


SECTOR_SIZE = 256
SECTORS_PER_TRACK = 16
TRACK_SIZE = SECTOR_SIZE * SECTORS_PER_TRACK
SAMPLE_BEFORE_PC = 8

# These are named byte-order candidates, not a claim that SunDog uses either
# filesystem or loader. Keeping the raw view separate preserves the original
# source offsets when a candidate view happens to contain the sample.
SECTOR_ORDER_CANDIDATES: dict[str, tuple[int, ...]] = {
    "raw-sequential": tuple(range(SECTORS_PER_TRACK)),
    "apple-pascal-2-ascending": (0, 8, 1, 9, 2, 10, 3, 11, 4, 12, 5, 13, 6, 14, 7, 15),
    "dos33-2-descending": (0, 7, 14, 6, 13, 5, 12, 4, 11, 3, 10, 2, 9, 1, 8, 15),
}

SAMPLE_RE = re.compile(
    r"^SUNDOG_HGR_CODE_SAMPLE window=(?P<window>\S+) page=(?P<page>\S+) "
    r"pc=\$(?P<pc>[0-9a-f]{4}) start=\$(?P<start>[0-9a-f]{4}) "
    r"length=(?P<length>\d+) bytes=(?P<bytes>[0-9a-f]+)$"
)


@dataclass(frozen=True)
class CodeSample:
    window: str
    page: str
    pc: int
    start: int
    payload: bytes

    @property
    def identity(self) -> tuple[str, str, int]:
        return self.window, self.page, self.pc


def sha256(payload: bytes) -> str:
    return hashlib.sha256(payload).hexdigest()


def parse_trace(payload: bytes) -> list[CodeSample]:
    """Parse first-writer code samples without retaining their text externally."""

    samples: dict[tuple[str, str, int], CodeSample] = {}
    for line in payload.decode("utf-8", errors="strict").splitlines():
        match = SAMPLE_RE.match(line)
        if not match:
            continue
        fields = match.groupdict()
        sample = CodeSample(
            window=fields["window"],
            page=fields["page"],
            pc=int(fields["pc"], 16),
            start=int(fields["start"], 16),
            payload=bytes.fromhex(fields["bytes"]),
        )
        declared_length = int(fields["length"])
        if declared_length != len(sample.payload):
            raise ValueError(
                f"sample {sample.identity} declares {declared_length} bytes, got {len(sample.payload)}"
            )
        if sample.start != sample.pc - SAMPLE_BEFORE_PC:
            raise ValueError(
                f"sample {sample.identity} start ${sample.start:04x} does not precede PC by {SAMPLE_BEFORE_PC}"
            )
        existing = samples.get(sample.identity)
        if existing and existing != sample:
            raise ValueError(f"sample {sample.identity} appears with different bytes")
        samples[sample.identity] = sample
    return [samples[key] for key in sorted(samples)]


def build_candidate_view(data: bytes, order: tuple[int, ...]) -> bytes:
    if len(data) % TRACK_SIZE:
        raise ValueError(f"disk size {len(data)} is not a whole number of 16-sector tracks")
    result = bytearray(len(data))
    destination = 0
    for track_start in range(0, len(data), TRACK_SIZE):
        for physical_sector in order:
            source = track_start + physical_sector * SECTOR_SIZE
            result[destination : destination + SECTOR_SIZE] = data[source : source + SECTOR_SIZE]
            destination += SECTOR_SIZE
    return bytes(result)


def all_occurrences(haystack: bytes, needle: bytes) -> Iterable[int]:
    start = 0
    while True:
        found = haystack.find(needle, start)
        if found < 0:
            return
        yield found
        start = found + 1


def source_segments(
    view_offset: int, length: int, order: tuple[int, ...]
) -> list[dict[str, int]]:
    """Map a candidate logical view span back to raw physical sectors."""

    result: list[dict[str, int]] = []
    remaining = length
    current = view_offset
    while remaining:
        track, in_track = divmod(current, TRACK_SIZE)
        logical_sector, sector_offset = divmod(in_track, SECTOR_SIZE)
        physical_sector = order[logical_sector]
        take = min(remaining, SECTOR_SIZE - sector_offset)
        raw_offset = track * TRACK_SIZE + physical_sector * SECTOR_SIZE + sector_offset
        if result and result[-1]["rawEndExclusive"] == raw_offset:
            result[-1]["rawEndExclusive"] += take
            result[-1]["length"] += take
        else:
            result.append(
                {
                    "track": track,
                    "logicalSector": logical_sector,
                    "physicalSector": physical_sector,
                    "rawOffset": raw_offset,
                    "rawEndExclusive": raw_offset + take,
                    "length": take,
                }
            )
        current += take
        remaining -= take
    return result


def match_sample(sample: CodeSample, disk_name: str, disk: bytes) -> list[dict[str, object]]:
    matches: list[dict[str, object]] = []
    for view_name, order in SECTOR_ORDER_CANDIDATES.items():
        view = build_candidate_view(disk, order)
        for offset in all_occurrences(view, sample.payload):
            matches.append(
                {
                    "disk": disk_name,
                    "candidateView": view_name,
                    "viewOffset": f"0x{offset:05x}",
                    "viewEndExclusive": f"0x{offset + len(sample.payload):05x}",
                    "sourceSegments": source_segments(offset, len(sample.payload), order),
                }
            )
    return matches


def build_report(trace: bytes, disk1_name: str, disk1: bytes, disk2_name: str, disk2: bytes) -> dict[str, object]:
    samples = parse_trace(trace)
    disks = ((disk1_name, disk1), (disk2_name, disk2))
    rows: list[dict[str, object]] = []
    for sample in samples:
        matches = [
            match
            for disk_name, disk in disks
            for match in match_sample(sample, disk_name, disk)
        ]
        rows.append(
            {
                "window": sample.window,
                "page": sample.page,
                "pc": f"0x{sample.pc:04x}",
                "runtimeStart": f"0x{sample.start:04x}",
                "runtimeLength": len(sample.payload),
                "runtimeSampleSha256": sha256(sample.payload),
                "exactByteMatches": matches,
            }
        )
    return {
        "schema": 1,
        "tool": "tools/apple2_runtime_code_provenance.py",
        "input": {
            "trace": {"size": len(trace), "sha256": sha256(trace), "sampleCount": len(samples)},
            "disk1": {"name": disk1_name, "size": len(disk1), "sha256": sha256(disk1)},
            "disk2": {"name": disk2_name, "size": len(disk2), "sha256": sha256(disk2)},
        },
        "samples": rows,
        "summary": {
            "samplesWithExactByteMatch": sum(bool(row["exactByteMatches"]) for row in rows),
            "samplesWithoutExactByteMatch": sum(not row["exactByteMatches"] for row in rows),
        },
        "interpretation": {
            "proven": [
                "MAME log 記錄的受限 runtime code window hash 與每個匹配候選的 DSK bytes 相等",
                "每個 candidate view offset 都可回推到未重排的 raw track／sector／byte segment",
            ],
            "strongInference": [
                "唯一且完整的 exact byte match 可作為後續 loader／renderer 研究的窄 source-provenance 線索",
            ],
            "unknown": [
                "match 不證明實際 loader、filesystem、函式邊界、renderer 語意或完整呼叫圖",
                "runtime PC／RAM window 不進 remake runtime、存檔、封包或玩家安裝需求",
            ],
        },
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("trace", type=Path, help="/tmp 中的 MAME log")
    parser.add_argument("disk1", type=Path, help="唯讀 Disk 1")
    parser.add_argument("disk2", type=Path, help="唯讀 Disk 2")
    parser.add_argument("--output", type=Path, help="可選 /tmp JSON 輸出；未指定時寫到 stdout")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    report = build_report(
        args.trace.read_bytes(),
        args.disk1.name,
        args.disk1.read_bytes(),
        args.disk2.name,
        args.disk2.read_bytes(),
    )
    encoded = json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(encoded, encoding="utf-8")
    else:
        print(encoded, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
