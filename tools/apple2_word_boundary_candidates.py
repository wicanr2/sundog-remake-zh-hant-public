#!/usr/bin/env python3
"""Report a high-bit/uppercase word-boundary candidate in an Apple II disk.

This is an evidence extractor, not a text decoder or a filesystem parser.  The
candidate rule is deliberately narrow: an ASCII letter whose high bit is set,
or an uppercase ASCII letter, is treated as the end of a word.  The raw bytes,
offsets and hashes are retained so a later code/data-flow investigation can
confirm or reject the interpretation.

The input may be a raw disk image or the Apple II source archive used by this
repository.  Archives are read in memory and are never extracted by this tool.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import zipfile
from pathlib import Path


DEFAULT_START = 0xB041
# The trailing byte at 0xB0FF starts the next, unproven fragment.  Keep the
# documented candidate slice end-exclusive at 0xB0FF until its consumer is
# identified.
DEFAULT_END = 0xB0FF


def parse_int(value: str) -> int:
    return int(value, 0)


def load_disk(path: Path) -> tuple[str, bytes, str | None, str | None]:
    """Return display name, disk bytes, archive name and archive hash."""

    file_bytes = path.read_bytes()
    archive_sha256 = hashlib.sha256(file_bytes).hexdigest()
    if zipfile.is_zipfile(path):
        with zipfile.ZipFile(path) as archive:
            members = [
                member
                for member in archive.namelist()
                if Path(member).name.upper() == "SUNDOG2.DSK"
            ]
            if len(members) != 1:
                raise ValueError(
                    f"expected exactly one SUNDOG2.DSK member, found {len(members)}"
                )
            return Path(members[0]).name, archive.read(members[0]), path.name, archive_sha256
    return path.name, file_bytes, None, None


def is_letter(value: int) -> bool:
    return 0x41 <= value <= 0x5A or 0x61 <= value <= 0x7A


def decode_candidates(raw: bytes, start: int, end: int) -> list[dict[str, object]]:
    entries: list[dict[str, object]] = []
    current: list[int] = []
    current_start: int | None = None

    for offset in range(start, end):
        value = raw[offset]
        ascii_value = value & 0x7F
        if not is_letter(ascii_value):
            if current:
                entries.append(make_entry(current_start, offset, current))
                current = []
                current_start = None
            continue

        if current_start is None:
            current_start = offset
        current.append(value)
        boundary = bool(value & 0x80) or 0x41 <= value <= 0x5A
        if boundary:
            entries.append(make_entry(current_start, offset + 1, current))
            current = []
            current_start = None

    if current:
        entries.append(make_entry(current_start, end, current))
    return entries


def make_entry(start: int | None, end: int, raw_values: list[int]) -> dict[str, object]:
    if start is None:
        raise ValueError("candidate has no start offset")
    source = "".join(chr(value & 0x7F).lower() for value in raw_values)
    return {
        "offset": f"0x{start:05x}",
        "endOffsetExclusive": f"0x{end:05x}",
        "length": len(raw_values),
        "source": source,
        "rawHex": " ".join(f"{value:02x}" for value in raw_values),
        "boundaryByte": f"0x{raw_values[-1]:02x}",
        "evidenceLevel": "strong-inference",
        "consumerStatus": "unknown",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("disk_or_archive", type=Path)
    parser.add_argument("--start", type=parse_int, default=DEFAULT_START)
    parser.add_argument("--end", type=parse_int, default=DEFAULT_END)
    args = parser.parse_args()

    display_name, raw, archive_name, archive_sha256 = load_disk(args.disk_or_archive)
    if not (0 <= args.start < args.end <= len(raw)):
        parser.error("region is outside the disk image")
    entries = decode_candidates(raw, args.start, args.end)
    result: dict[str, object] = {
        "schema": 1,
        "tool": "apple2_word_boundary_candidates",
        "disk": display_name,
        "diskSize": len(raw),
        "diskSha256": hashlib.sha256(raw).hexdigest(),
        "sourceInput": args.disk_or_archive.name,
        "region": {
            "start": f"0x{args.start:05x}",
            "end": f"0x{args.end:05x}",
            "boundaryRule": [
                "ASCII uppercase letter ends a candidate word",
                "high-bit ASCII letter ends a candidate word",
            ],
            "evidenceLevel": "strong-inference",
            "consumerStatus": "unknown",
        },
        "entries": entries,
    }
    if archive_name is not None and archive_sha256 is not None:
        result["archive"] = {
            "name": archive_name,
            "sha256": archive_sha256,
        }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
