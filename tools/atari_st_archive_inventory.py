#!/usr/bin/env python3
"""盤點 Atari ST SunDog 外層／內層 archive，不解碼或改寫 STX 資料。"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def member_record(info: zipfile.ZipInfo, data: bytes | None = None) -> dict[str, object]:
    record: dict[str, object] = {
        "name": info.filename,
        "compressedSize": info.compress_size,
        "uncompressedSize": info.file_size,
        "isDirectory": info.is_dir(),
    }
    if data is not None:
        record["sha256"] = digest(data)
        record["headHex"] = data[:64].hex()
    return record


def inventory(path: Path) -> dict[str, object]:
    archive_bytes = path.read_bytes()
    result: dict[str, object] = {
        "input": str(path),
        "inputSha256": digest(archive_bytes),
        "inputSize": len(archive_bytes),
        "members": [],
    }
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        members: list[dict[str, object]] = []
        for info in outer.infolist():
            data = None if info.is_dir() else outer.read(info)
            record = member_record(info, data)
            if data is not None and info.filename.lower().endswith(".zip"):
                try:
                    with zipfile.ZipFile(io.BytesIO(data)) as nested:
                        nested_members = []
                        for nested_info in nested.infolist():
                            nested_data = (
                                None
                                if nested_info.is_dir()
                                else nested.read(nested_info)
                            )
                            nested_members.append(member_record(nested_info, nested_data))
                        record["nestedMembers"] = nested_members
                except zipfile.BadZipFile:
                    record["nestedArchiveError"] = "BadZipFile"
            members.append(record)
        result["members"] = members
    return result


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    return parser.parse_args()


def main() -> None:
    print(json.dumps(inventory(parse_args().archive), ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
