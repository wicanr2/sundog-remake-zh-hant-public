#!/usr/bin/env python3
"""從自備 Atari ST 磁碟解碼十八張行星地表；契約見 ground-route-spec.md。"""
import argparse
import hashlib
import json
from pathlib import Path

DISK_SHA256 = "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6"
TRANSITIONS = [None] * 14
for states, values in [((1, 4, 9, 13), (2, 3, 10, 13)),
                       ((2, 7, 11), (1, 8, 11, 11)),
                       ((3, 6, 12), (4, 5, 12, 12)),
                       ((5, 8, 10, 0), (6, 7, 9, 0))]:
    for state in states:
        TRANSITIONS[state] = values


def decode_record(record):
    if len(record) != 256:
        raise ValueError("行星記錄必須恰為 256 bytes")
    rows = []
    for y in range(24):
        row, state = [], 0
        for x in range(40):
            pos = 16 + y * 10 + (x // 8) * 2
            symbol = (int.from_bytes(record[pos:pos + 2], "big") >> (x % 8 * 2)) & 3
            state = TRANSITIONS[state][symbol]
            row.append(state)
        rows.append(row)
    return rows


def decode_disk(disk):
    if hashlib.sha256(disk).hexdigest() != DISK_SHA256:
        raise ValueError("磁碟身分不符已審查版本")
    records = []
    for index in range(18):
        offset = 0xaa00 + index * 256
        record = disk[offset:offset + 256]
        records.append(dict(planetIndex=index, rawOffset=offset,
                            header=list(record[:16]), states=decode_record(record)))
    return dict(schema=1, diskSHA256=DISK_SHA256, maps=records)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("disk", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    args.output.write_text(json.dumps(decode_disk(args.disk.read_bytes()), ensure_ascii=False,
                                      separators=(",", ":")) + "\n", encoding="utf-8")
