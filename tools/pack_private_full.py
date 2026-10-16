#!/usr/bin/env python3
"""替私人完整版寫說明、逐檔 manifest，再打成一份 UTF-8 檔名的 ZIP。

檔名有中文，所以用 :mod:`zipfile` 而不是 ``zip``：Python 對非 ASCII 檔名一定會設
UTF-8 旗標，central 與 local header 兩邊都設。
"""

from __future__ import annotations

import argparse
import hashlib
import re
import tempfile
import shutil
import zipfile
from pathlib import Path

NOTES = """SunDog（陽犬號）remake {version} 私人完整版

本目錄包含 Linux、Windows、macOS 三平台 release、玩家自備的 Atari ST 原版磁碟
`sundog.st`，以及由該磁碟匯出的音效。原版磁碟與音效仍受原權利人保護，只供擁有合法
原版的使用者在本機使用；不得公開上傳、再散布或併入 GitHub Release。

Linux 請執行「啟動陽犬號.sh」，會自動指定原版磁碟與音效；可傳入視窗與保存參數。
Windows 與 macOS 請先解壓各自 ZIP，再依包內 README 啟動並指定本目錄的原版磁碟。
另附三份各平台私人完整版 ZIP，已展開引擎並附啟動腳本、完整原版磁碟與 33 段音效。

只支援 schema44 新存檔；schema43 與更早存檔不支援，不提供遷移。
F8 依序切換 Atari ST、Atari ST HD、Apple II，再回 Atari ST。HD 保留 Atari ST
操作與分區，使用獨立高解析重畫；F2 切換四語，視窗可調整大小。
Windows 套件可在 Wine 驗啟動；macOS 為雙架構靜態驗證，尚未完成實機驗收。

`MANIFEST.sha256` 記錄此目錄所有交付檔案的 SHA-256；外層 ZIP 的雜湊另記在
`../SHA256SUMS`。
"""


def digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def platform_full(stage: Path, output: Path, version: str, platform: str) -> Path:
    """建立可直接解壓啟動的各平台本機包；原版來源仍只留本機。"""
    name = f"SunDog-{version}-{platform}-private-full"
    archive = output / f"{name}.zip"
    if archive.exists():
        raise SystemExit(f"封包已存在，不可覆寫：{archive}")
    with tempfile.TemporaryDirectory(prefix="sundog-private-platform-") as temporary:
        root = Path(temporary) / name
        root.mkdir()
        if platform == "linux-x86_64":
            shutil.copy2(stage / f"SunDog-{version}-{platform}.AppImage", root)
            shutil.copy2(stage / "啟動陽犬號.sh", root)
        else:
            with zipfile.ZipFile(stage / f"SunDog-{version}-{platform}.zip") as engine:
                for entry in engine.infolist():
                    relative = Path(entry.filename)
                    if relative.is_absolute() or ".." in relative.parts:
                        raise SystemExit("引擎封包路徑越界")
                engine.extractall(root)
                for entry in engine.infolist():
                    path = root / entry.filename
                    if path.is_file():
                        path.chmod((entry.external_attr >> 16) & 0o777 or 0o644)
            if platform == "windows-amd64":
                executable = next(root.rglob("sundog-remake.exe")).relative_to(root)
                (root / "Launch-SunDog.cmd").write_bytes((
                    '@echo off\r\n'
                    f'"%~dp0{str(executable).replace(chr(47), chr(92))}" '
                    '-atari-disk "%~dp0sundog.st" -audio "%~dp0audio" %*\r\n'
                ).encode("ascii"))
            else:
                executable = next(root.rglob("SunDog.app/Contents/MacOS/sundog-remake")).relative_to(root)
                launcher = root / "Launch-SunDog.command"
                launcher.write_text(
                    '#!/bin/sh\nset -eu\n'
                    'here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\n'
                    f'exec "$here/{executable.as_posix()}" '
                    '-atari-disk "$here/sundog.st" -audio "$here/audio" "$@"\n',
                    encoding="utf-8")
                launcher.chmod(0o755)
        shutil.copy2(stage / "sundog.st", root)
        shutil.copytree(stage / "audio", root / "audio")
        (root / "說明-私人完整版.txt").write_text(
            NOTES.format(version=version) +
            "\n本包已展開本平台引擎。Linux 執行啟動陽犬號.sh；Windows 執行 "
            "Launch-SunDog.cmd；macOS 執行 Launch-SunDog.command。\n",
            encoding="utf-8")
        payload = sorted(p for p in root.rglob("*") if p.is_file())
        (root / "MANIFEST.sha256").write_text(
            "".join(f"{digest(p)}  ./{p.relative_to(root).as_posix()}\n" for p in payload),
            encoding="utf-8")
        with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as bundle:
            for path in sorted(root.rglob("*")):
                if path.is_file():
                    bundle.write(path, f"{name}/{path.relative_to(root).as_posix()}")
    return archive


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--stage", required=True, type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    if not re.fullmatch(r"v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}", args.version):
        raise SystemExit("版本格式不符")
    archive = args.output / f"{args.stage.name}.zip"
    if archive.exists():
        raise SystemExit("封包已存在，不可覆寫")

    stage: Path = args.stage
    launcher = stage / "啟動陽犬號.sh"
    launcher.write_text(
        '#!/bin/sh\nset -eu\n'
        'here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\n'
        f'exec "$here/SunDog-{args.version}-linux-x86_64.AppImage" '
        '--appimage-extract-and-run -atari-disk "$here/sundog.st" -audio "$here/audio" "$@"\n',
        encoding="utf-8")
    launcher.chmod(0o755)
    (stage / "說明-私人完整版.txt").write_text(
        NOTES.format(version=args.version), encoding="utf-8")

    payload = sorted(
        path for path in stage.rglob("*")
        if path.is_file() and path.name != "MANIFEST.sha256"
    )
    lines = [f"{digest(path)}  ./{path.relative_to(stage).as_posix()}" for path in payload]
    (stage / "MANIFEST.sha256").write_text("\n".join(lines) + "\n", encoding="utf-8")

    archive = args.output / f"{stage.name}.zip"
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as bundle:
        for path in sorted(stage.rglob("*")):
            if path.is_file():
                bundle.write(path, f"{stage.name}/{path.relative_to(stage).as_posix()}")

    # 打完之後把 local header 也讀一遍：中文檔名沒設 UTF-8 旗標的話，Windows 的
    # 內建解壓縮會解出亂碼目錄名。
    with zipfile.ZipFile(archive) as bundle:
        for info in bundle.infolist():
            if not info.filename.isascii() and not info.flag_bits & 0x800:
                raise SystemExit(f"{info.filename} 沒有 UTF-8 旗標")
    print(f"{digest(archive)}  {archive.name}")
    archives = [archive]
    for platform in ("linux-x86_64", "windows-amd64", "macos-universal"):
        archives.append(platform_full(stage, args.output, args.version, platform))
    (args.output / "SHA256SUMS").write_text(
        "".join(f"{digest(path)}  {path.name}\n" for path in archives), encoding="utf-8")


if __name__ == "__main__":
    main()
