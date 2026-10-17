#!/usr/bin/env python3
"""把已編好的三個平台執行檔包成可散布的套件。

輸入是 `tools/build_release.sh` 編出來的 staging 目錄；本工具只做封裝，不編譯。
三個平台共用同一份酬載（字型、五語 catalog／對話、操作說明），差別在外殼：

* Linux：AppDir ＋ appimagetool → 單一 AppImage
* Windows：zip，**每一筆都打開 UTF-8 檔名旗標**，中文檔名在任何解壓工具下都一致
* macOS：`.app` bundle 再 zip，保留執行位元

Windows 那份**沒有隨包的 DLL**：執行檔是純 Go（沒有 cgo），匯入表只有 `kernel32.dll`，
其餘（`opengl32`、`d3d11`、`user32`…）都是 Windows 自帶的系統元件，執行時才動態載入。
本工具會檢查匯入表，出現非系統 DLL 就中止，免得「其實需要卻沒包」被靜靜放過。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import stat
import struct
import subprocess
import zipfile
from pathlib import Path


# 隨執行檔一起散布的唯讀檔。左邊是套件裡的相對路徑，右邊是 repo 裡的來源。
PAYLOAD = {
    # 授權條款要在每一個發行包裡：拿到套件的人看不到儲存庫。
    "LICENSE": "LICENSE",
    "assets/fonts/NotoSansCJKtc-Regular.otf": "assets/fonts/NotoSansCJKtc-Regular.otf",
    "assets/fonts/LICENSE_OFL.txt": "assets/fonts/LICENSE_OFL.txt",
    "assets/fonts/ASSET-MANIFEST.md": "assets/fonts/ASSET-MANIFEST.md",
    "docs/i18n/apple2-text-catalog.json": "docs/i18n/apple2-text-catalog.json",
    "docs/i18n/apple2-text-zh-Hans.json": "docs/i18n/apple2-text-zh-Hans.json",
    "docs/i18n/apple2-text-ja.json": "docs/i18n/apple2-text-ja.json",
    "docs/i18n/apple2-text-en.json": "docs/i18n/apple2-text-en.json",
    "docs/i18n/sundog-ui-zh-Hant.json": "docs/i18n/sundog-ui-zh-Hant.json",
    "docs/i18n/sundog-ui-zh-Hans.json": "docs/i18n/sundog-ui-zh-Hans.json",
    "docs/i18n/sundog-ui-ja.json": "docs/i18n/sundog-ui-ja.json",
    "docs/i18n/sundog-ui-en.json": "docs/i18n/sundog-ui-en.json",
    "docs/i18n/sundog-ui-ko.json": "docs/i18n/sundog-ui-ko.json",
    "docs/i18n/apple2-text-ko.json": "docs/i18n/apple2-text-ko.json",
    "docs/i18n/atari-st-dialog-zh-Hant.json": "docs/i18n/atari-st-dialog-zh-Hant.json",
    "docs/i18n/atari-st-dialog-zh-Hans.json": "docs/i18n/atari-st-dialog-zh-Hans.json",
    "docs/i18n/atari-st-dialog-ja.json": "docs/i18n/atari-st-dialog-ja.json",
    "docs/i18n/atari-st-dialog-en.json": "docs/i18n/atari-st-dialog-en.json",
    "docs/i18n/atari-st-dialog-ko.json": "docs/i18n/atari-st-dialog-ko.json",
    "操作說明.md": "docs/manual/remake-controls-zh-Hant.md",
}

# Windows 執行檔的匯入表只允許這些。純 Go 的建置目前只會有 kernel32。
SYSTEM_DLLS = {
    "kernel32.dll", "user32.dll", "gdi32.dll", "advapi32.dll", "shell32.dll",
    "ole32.dll", "oleaut32.dll", "ws2_32.dll", "winmm.dll", "imm32.dll",
    "opengl32.dll", "dwmapi.dll", "shcore.dll", "ntdll.dll", "msvcrt.dll",
    "bcrypt.dll", "crypt32.dll", "psapi.dll", "userenv.dll", "version.dll",
}

APP_NAME = "SunDog"
BUNDLE_ID = "tw.wicanr2.sundog-remake"
VERSION_RE = re.compile(r"^v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}$")


class ReleaseError(RuntimeError):
    """套件的內容或結構不符合本工具宣稱的版面。"""


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def windows_imports(path: Path) -> list[str]:
    """讀 PE 的匯入表。只解析需要的那幾個欄位，不引外部套件。"""
    data = path.read_bytes()
    pe = struct.unpack_from("<I", data, 0x3C)[0]
    if data[pe:pe + 4] != b"PE\0\0":
        raise ReleaseError(f"{path.name} 不是 PE 檔")
    sections = struct.unpack_from("<H", data, pe + 6)[0]
    optional_size = struct.unpack_from("<H", data, pe + 20)[0]
    optional = pe + 24
    magic = struct.unpack_from("<H", data, optional)[0]
    directories = optional + (112 if magic == 0x20B else 96)
    import_rva = struct.unpack_from("<I", data, directories + 8)[0]

    table = []
    for index in range(sections):
        entry = optional + optional_size + index * 40
        virtual_size, virtual_address, raw_size, raw_pointer = struct.unpack_from("<IIII", data, entry + 8)
        table.append((virtual_address, max(virtual_size, raw_size), raw_pointer))

    def to_offset(rva: int) -> int:
        for virtual_address, size, raw_pointer in table:
            if virtual_address <= rva < virtual_address + size:
                return raw_pointer + (rva - virtual_address)
        raise ReleaseError(f"RVA 0x{rva:x} 不在任何區段裡")

    names: list[str] = []
    cursor = to_offset(import_rva)
    while True:
        descriptor = data[cursor:cursor + 20]
        if len(descriptor) < 20 or descriptor == b"\0" * 20:
            break
        name_rva = struct.unpack_from("<I", descriptor, 12)[0]
        if name_rva == 0:
            break
        start = to_offset(name_rva)
        names.append(data[start:data.index(b"\0", start)].decode("ascii"))
        cursor += 20
    return names


def copy_payload(repo: Path, target: Path) -> None:
    for relative, source in PAYLOAD.items():
        destination = target / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(repo / source, destination)


def write_readme(target: Path, version: str, platform: str, launch: str, notes: list[str]) -> None:
    lines = [
        f"# 陽犬號 SunDog 五語系重製版 {version}",
        "",
        f"平台：{platform}",
        "",
        "## 執行",
        "",
        launch,
        "",
        "## 操作",
        "",
        "鍵盤與滑鼠對照見同目錄的 `操作說明.md`。`F8` 依序切換 Atari ST、Atari ST HD、Apple II，"
        "再回 Atari ST；HD 使用獨立高解析重畫，保持 Atari ST 操作與分區。"
        "`F2`（或標題畫面的「選項」）開選項卡，也能在裡面指定或偵測你自己的 Atari ST 磁碟，"
        "選了就記在設定檔，下次啟動沿用。把 `.st` 放在執行檔旁邊或設定目錄，第一次啟動就會自動找到，並以 Atari ST 的樣式開場。",
        "",
        "## 授權",
        "",
        "本套件依復古重製 source-available 授權條款 1.0（RRSAL-1.0）授權：非商業用途免費"
        "（含修改與再散布），實況、影片與評論明示允許（含平台分潤），商業用途要先與著作權人"
        "洽談（wicanr2@gmail.com）。條款全文與不涵蓋原版素材的說明見同目錄的 `LICENSE`。",
        "",
        "## 存檔位置",
        "",
        "存檔、角色保存庫與設定檔（`settings.json`）寫在使用者自己的設定目錄底下的 `sundog-remake/`，",
        "跟你從哪個目錄啟動無關：",
        "",
        "- Linux：`~/.config/sundog-remake/`",
        "- Windows：`%APPDATA%\\sundog-remake\\`",
        "- macOS：`~/Library/Application Support/sundog-remake/`",
        "",
        "要放在別的地方，用 `-save`、`-library` 與 `-settings` 指定路徑。",
        "只支援 schema45 新存檔；舊存檔與原版 Library Disk 不支援，不提供遷移。",
        "",
        "## 邊界",
        "",
        "這是研究性重製版，不含任何原版的程式碼、圖像或音訊。原版 1985 年 FTL Games／",
        "Software Heaven，著作權屬原權利人。介面美術與圖塊全部是重製版自己畫的。",
        "",
        "支援從玩家自備磁碟匯出的原版音效；尚未匯入時不會播放，設定方式見下方音效說明。未提供額外配樂。",
    ]
    if notes:
        lines += ["", "## 這個平台的注意事項", ""] + [f"- {note}" for note in notes]
    (target / "README.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def build_icon(repo: Path, destination: Path) -> None:
    """複製已經產好的應用程式圖示。

    圖示由 `tools/build_app_icon.py` 產生並進版控，封裝時不再跑影像處理——
    打包用的 image 有 `file`（appimagetool 要它）但沒有 PIL，兩者不在同一個 image 裡。
    """
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(repo / "assets/icons/sundog-remake.png", destination)


UTF8_NAME_FLAG = 0x800
LOCAL_HEADER = b"PK\x03\x04"
CENTRAL_HEADER = b"PK\x01\x02"
END_OF_CENTRAL_DIRECTORY = b"PK\x05\x06"


def zip_tree(root: Path, archive: Path, prefix: str) -> None:
    """把目錄打成 zip，保留執行位元，並在每一筆打開 UTF-8 檔名旗標。"""
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as bundle:
        for path in sorted(root.rglob("*")):
            relative = Path(prefix) / path.relative_to(root)
            info = zipfile.ZipInfo(str(relative) + ("/" if path.is_dir() else ""))
            mode = path.stat().st_mode
            info.external_attr = (mode & 0xFFFF) << 16
            if path.is_dir():
                info.external_attr |= 0x10
                bundle.writestr(info, b"")
                continue
            info.compress_type = zipfile.ZIP_DEFLATED
            bundle.writestr(info, path.read_bytes())
    force_utf8_flag(archive)


def force_utf8_flag(archive: Path) -> None:
    """在寫好的 zip 上，把 general purpose bit 11（UTF-8 檔名）補進兩種標頭。

    不能在 `ZipInfo` 上設就算數：`ZipFile._open_to_write` 會先把 `flag_bits` 歸零，
    所以本地標頭寫出去時旗標已經不見了。Python 只有在檔名不是 ASCII 時才自己補這一位，
    而純 ASCII 的檔名同樣是合法的 UTF-8——一律宣告，解壓端就不必猜編碼。
    """
    data = bytearray(archive.read_bytes())

    end = data.rfind(END_OF_CENTRAL_DIRECTORY)
    if end < 0:
        raise ReleaseError(f"{archive.name} 找不到 end of central directory")
    entries = struct.unpack_from("<H", data, end + 10)[0]
    cursor = struct.unpack_from("<I", data, end + 16)[0]

    for _ in range(entries):
        if data[cursor:cursor + 4] != CENTRAL_HEADER:
            raise ReleaseError(f"{archive.name} 的 central directory 版面不符預期")
        flags = struct.unpack_from("<H", data, cursor + 8)[0]
        struct.pack_into("<H", data, cursor + 8, flags | UTF8_NAME_FLAG)

        local = struct.unpack_from("<I", data, cursor + 42)[0]
        if data[local:local + 4] != LOCAL_HEADER:
            raise ReleaseError(f"{archive.name} 的 local header 位移不符預期")
        local_flags = struct.unpack_from("<H", data, local + 6)[0]
        struct.pack_into("<H", data, local + 6, local_flags | UTF8_NAME_FLAG)

        name_length, extra_length, comment_length = struct.unpack_from("<HHH", data, cursor + 28)
        cursor += 46 + name_length + extra_length + comment_length

    archive.write_bytes(bytes(data))


def verify_zip_utf8(archive: Path) -> None:
    """兩種標頭都要帶旗標。只查 central directory 會漏掉本地標頭。"""
    data = archive.read_bytes()
    with zipfile.ZipFile(archive) as bundle:
        names = bundle.namelist()
        for info in bundle.infolist():
            if not info.flag_bits & UTF8_NAME_FLAG:
                raise ReleaseError(f"{archive.name} 的 {info.filename} central header 沒有 UTF-8 旗標")
            local_flags = struct.unpack_from("<H", data, info.header_offset + 6)[0]
            if not local_flags & UTF8_NAME_FLAG:
                raise ReleaseError(f"{archive.name} 的 {info.filename} local header 沒有 UTF-8 旗標")
    if not names:
        raise ReleaseError(f"{archive.name} 是空的")


def package_windows(repo: Path, staging: Path, output: Path, version: str) -> Path:
    binary = staging / "bin" / "sundog-remake.exe"
    imports = windows_imports(binary)
    unexpected = sorted({name for name in imports if name.lower() not in SYSTEM_DLLS})
    if unexpected:
        raise ReleaseError(f"Windows 執行檔匯入了非系統 DLL，必須隨包散布：{unexpected}")

    root = output / "staging-windows" / f"{APP_NAME}-{version}-windows-amd64"
    if root.exists():
        shutil.rmtree(root)
    root.mkdir(parents=True)
    shutil.copy2(binary, root / "sundog-remake.exe")
    shutil.copy2(staging / "bin" / "sundog-sound-export.exe", root / "sundog-sound-export.exe")
    copy_payload(repo, root)
    write_readme(
        root, version, "Windows x64",
        "雙擊 `sundog-remake.exe`。整個資料夾要一起解壓，執行檔會在自己旁邊找字型與文字檔。",
        [
            f"執行檔是純 Go 建置，匯入表只有 {'、'.join(sorted(set(imports)))}；"
            "其餘用到的（`opengl32`、`d3d11`、`user32` 等）都是 Windows 自帶的系統元件，"
            "**沒有需要隨包散布的 DLL**。",
            "壓縮檔的每一筆都打開了 UTF-8 檔名旗標，中文檔名不需要指定編碼。",
            "`sundog-sound-export.exe` 從你自己的原版磁碟轉出音效；沒有轉過的話遊戲是靜音的。",
        ],
    )
    archive = output / f"{APP_NAME}-{version}-windows-amd64.zip"
    zip_tree(root, archive, root.name)
    verify_zip_utf8(archive)
    shutil.rmtree(root.parent)
    return archive


def package_macos(repo: Path, staging: Path, output: Path, version: str) -> Path:
    root = output / "staging-macos"
    if root.exists():
        shutil.rmtree(root)
    app = root / f"{APP_NAME}.app"
    macos_dir = app / "Contents" / "MacOS"
    resources = app / "Contents" / "Resources"
    macos_dir.mkdir(parents=True)
    resources.mkdir(parents=True)

    binary = macos_dir / "sundog-remake"
    shutil.copy2(staging / "bin" / "sundog-remake-macos-universal", binary)
    binary.chmod(binary.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    exporter = macos_dir / "sundog-sound-export"
    shutil.copy2(staging / "bin" / "sundog-sound-export-macos-universal", exporter)
    exporter.chmod(exporter.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    copy_payload(repo, resources)
    write_readme(
        resources, version, "macOS（universal：Apple Silicon 與 Intel）",
        "把 `SunDog.app` 拖進「應用程式」再開啟。",
        [
            "這份 bundle **沒有簽章也沒有經過 Apple 公證**（notarization），因為它是在 Linux 上"
            "交叉編出來的。第一次開啟要用「右鍵 → 打開」，或到「系統設定 → 隱私權與安全性」按"
            "「仍要打開」。",
            "arm64 那一弧帶有 ad-hoc 簽章，所以在 Apple Silicon 上不會被系統直接終止。",
            "沒有 Mac 可以實測，所以只做過靜態檢查：雙架構、arm64 有簽章、相依只在 "
            "`/usr/lib` 與 `/System/Library`。實際遊玩驗證尚未進行。",
        ],
    )

    plist = f"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>{APP_NAME}</string>
  <key>CFBundleDisplayName</key><string>陽犬號 SunDog</string>
  <key>CFBundleIdentifier</key><string>{BUNDLE_ID}</string>
  <key>CFBundleVersion</key><string>{version}</string>
  <key>CFBundleShortVersionString</key><string>{version}</string>
  <key>CFBundleExecutable</key><string>sundog-remake</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleIconFile</key><string>icon.png</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
"""
    (app / "Contents" / "Info.plist").write_text(plist, encoding="utf-8")
    build_icon(repo, resources / "icon.png")

    archive = output / f"{APP_NAME}-{version}-macos-universal.zip"
    zip_tree(root, archive, "")
    verify_zip_utf8(archive)
    shutil.rmtree(root)
    return archive


def package_linux(repo: Path, staging: Path, output: Path, version: str) -> Path:
    appdir = output / "staging-linux" / f"{APP_NAME}.AppDir"
    if appdir.parent.exists():
        shutil.rmtree(appdir.parent)
    (appdir / "usr" / "bin").mkdir(parents=True)

    binary = appdir / "usr" / "bin" / "sundog-remake"
    shutil.copy2(staging / "bin" / "sundog-remake-linux-amd64", binary)
    binary.chmod(binary.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    exporter = appdir / "usr" / "bin" / "sundog-sound-export"
    shutil.copy2(staging / "bin" / "sundog-sound-export-linux-amd64", exporter)
    exporter.chmod(exporter.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    # 酬載放在執行檔旁邊，appdir.Asset 才找得到。
    copy_payload(repo, appdir / "usr" / "bin")
    doc_dir = appdir / "usr" / "share" / "doc" / "sundog-remake"
    doc_dir.mkdir(parents=True)
    shutil.copy2(repo / "LICENSE", doc_dir / "LICENSE")
    write_readme(
        appdir / "usr" / "bin", version, "Linux x86_64",
        "`chmod +x` 之後直接執行這個 AppImage。需要 X11 或 XWayland 與 OpenGL 2.1 以上。",
        [
            "AppImage 內部是唯讀掛載，所以存檔一律寫到 `~/.config/sundog-remake/`。",
            "沒有 FUSE 的環境可以用 `--appimage-extract-and-run` 執行。",
            "AppImage 裡附了 `sundog-sound-export`，從你自己的原版磁碟轉出音效；"
            "用 `--appimage-extract` 取出後執行，沒有轉過的話遊戲是靜音的。",
        ],
    )

    build_icon(repo, appdir / "sundog-remake.png")
    desktop = f"""[Desktop Entry]
Type=Application
Name={APP_NAME}
Name[zh_TW]=陽犬號 SunDog
Comment=SunDog: Frozen Legacy 的繁體中文重製版
Exec=sundog-remake
Icon=sundog-remake
Categories=Game;AdventureGame;
Terminal=false
"""
    (appdir / "sundog-remake.desktop").write_text(desktop, encoding="utf-8")

    # AppRun 換到執行檔目錄再啟動，資產就在旁邊；工作目錄留給存檔的 fallback 判斷。
    apprun = """#!/bin/sh
HERE=$(dirname "$(readlink -f "$0")")
exec "$HERE/usr/bin/sundog-remake" "$@"
"""
    run_path = appdir / "AppRun"
    run_path.write_text(apprun, encoding="utf-8")
    run_path.chmod(0o755)

    archive = output / f"{APP_NAME}-{version}-linux-x86_64.AppImage"
    environment = dict(os.environ, ARCH="x86_64")
    result = subprocess.run(
        [
            "/appimage/appimagetool.AppImage", "--appimage-extract-and-run",
            "--runtime-file", "/appimage/runtime-x86_64",
            str(appdir), str(archive),
        ],
        env=environment, capture_output=True, text=True,
    )
    if result.returncode != 0:
        # 失敗時把完整輸出帶出來。appimagetool 的實際成因都寫在 stderr，
        # 只回報 exit code 會逼下一個人重跑一次才看得到。
        raise ReleaseError(
            "appimagetool 失敗（exit %d）\n--- stdout ---\n%s\n--- stderr ---\n%s"
            % (result.returncode, result.stdout, result.stderr)
        )
    archive.chmod(archive.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    shutil.rmtree(appdir.parent)
    return archive


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--staging", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()

    if not VERSION_RE.fullmatch(args.version):
        raise ReleaseError(f"版號不符合 v.<主>.<次>.<修訂>-YYYYMMDD：{args.version}")

    args.output.mkdir(parents=True, exist_ok=True)
    produced = [
        package_linux(args.repo, args.staging, args.output, args.version),
        package_windows(args.repo, args.staging, args.output, args.version),
        package_macos(args.repo, args.staging, args.output, args.version),
    ]

    checksums = args.output / "SHA256SUMS"
    checksums.write_text(
        "".join(f"{sha256(path)}  {path.name}\n" for path in sorted(produced, key=lambda p: p.name)),
        encoding="utf-8",
    )
    print(json.dumps(
        {
            "status": "pass",
            "version": args.version,
            "packages": [
                {"name": path.name, "bytes": path.stat().st_size, "sha256": sha256(path)}
                for path in produced
            ],
        },
        ensure_ascii=False, indent=2,
    ))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
