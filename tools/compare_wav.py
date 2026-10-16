#!/usr/bin/env python3
"""Compare two directories of mono 16-bit WAV files sample for sample."""
import struct
import sys
from pathlib import Path


def read_wav(path):
    raw = path.read_bytes()
    if raw[:4] != b"RIFF" or raw[8:12] != b"WAVE":
        raise ValueError(f"{path} is not a WAV file")
    cursor = 12
    body = None
    while cursor + 8 <= len(raw):
        name = raw[cursor:cursor + 4]
        size = struct.unpack_from("<I", raw, cursor + 4)[0]
        chunk = raw[cursor + 8:cursor + 8 + size]
        if name == b"data":
            body = chunk
            break
        cursor += 8 + size + (size & 1)
    if body is None:
        raise ValueError(f"{path} has no data chunk")
    count = len(body) // 2
    return struct.unpack(f"<{count}h", body[:count * 2])


def main(ours_dir, theirs_dir):
    ours_dir, theirs_dir = Path(ours_dir), Path(theirs_dir)
    names = sorted(p.stem for p in ours_dir.glob("*.wav"))
    if not names:
        raise SystemExit(f"no WAV files in {ours_dir}")

    worst = 0
    failures = []
    for name in names:
        theirs_path = theirs_dir / f"{name}.wav"
        if not theirs_path.exists():
            failures.append(f"{name}: missing from the reference output")
            continue
        ours = read_wav(ours_dir / f"{name}.wav")
        theirs = read_wav(theirs_path)
        if len(ours) != len(theirs):
            failures.append(
                f"{name}: {len(ours)} samples against {len(theirs)}")
            continue
        difference = max((abs(a - b) for a, b in zip(ours, theirs)), default=0)
        worst = max(worst, difference)
        status = "same" if difference == 0 else f"differs by {difference}"
        print(f"{name:<26} {len(ours):>8} samples  {status}")
        if difference:
            failures.append(f"{name}: differs by up to {difference}")

    print()
    if failures:
        print(f"{len(failures)} of {len(names)} sounds differ:")
        for failure in failures:
            print(f"  {failure}")
        return 1
    print(f"all {len(names)} sounds match the reference exactly")
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: compare_wav.py <ours-dir> <theirs-dir>")
    sys.exit(main(sys.argv[1], sys.argv[2]))
