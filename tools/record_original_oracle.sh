#!/usr/bin/env bash
set -euo pipefail

# 錄原版 Atari ST 的開機到主選單，供推廣影片的並列對照使用。
# 規格：docs/spec/promo-video.md
#
# 產物含原版畫面：只寫到指定的輸出目錄，不進版控、不進 release。
#
# 用法：tools/record_original_oracle.sh /絕對/輸出目錄 /raw/disk.st /tos.img
#
# [HARD] 錄影一定要用 --run-vbls 上界，而且用 png codec。
#   bmp codec 是每幀 768 KB 未壓縮：一次沒有上界的 bmp 錄影在本機產生過 12 GB 檔案。
#   png codec 約每幀 7.6 KB，慢但可控。
#
# 成本（實測）：png codec 約 34 VBL/s，開機走到主選單約 9000 VBL ≈ 4.5 分鐘、約 70 MB。

out_dir=${1:?用法：record_original_oracle.sh /絕對/輸出目錄 /raw/disk.st /tos.img}
disk=${2:?需要 raw .st 磁碟路徑}
tos=${3:?需要 TOS ROM 路徑}

ORACLE_IMAGE=${SUNDOG_ORACLE_IMAGE:-sundog-atari-st-oracle:20260812}
MACHINE=${SUNDOG_ORACLE_MACHINE:-megaste}
VBLS=${SUNDOG_ORACLE_VBLS:-11000}
DISPLAY_NUMBER=${SUNDOG_ORACLE_DISPLAY:-97}

if [[ "$out_dir" != /* ]]; then
	echo "輸出目錄必須是絕對路徑。" >&2
	exit 2
fi
test -f "$disk"
test -f "$tos"
mkdir -p "$out_dir"

disk_dir=$(cd "$(dirname "$disk")" && pwd)
tos_dir=$(cd "$(dirname "$tos")" && pwd)

docker run --rm --network none \
	--log-opt max-size=10m --log-opt max-file=3 \
	-v "$disk_dir":/disk:ro -v "$tos_dir":/tos:ro -v "$out_dir":/out \
	--entrypoint sh "$ORACLE_IMAGE" -c "
set -eu
Xvfb :$DISPLAY_NUMBER -screen 0 1024x768x24 -nolisten tcp >/tmp/xvfb.log 2>&1 &
sleep 3
export DISPLAY=:$DISPLAY_NUMBER
# 磁碟一律先複製成可丟棄的副本，原始媒體維持唯讀。
cp '/disk/$(basename "$disk")' /tmp/disk.st
hatari --machine $MACHINE --tos '/tos/$(basename "$tos")' --disk-a /tmp/disk.st \\
	--auto 'A:\\SUNDOG.PRG' --sound off --fast-boot on --fast-forward on \\
	--statusbar off --drive-led off --borders off --crop on \\
	--avirecord --avi-vcodec png --png-level 6 --avi-file /out/original-boot.avi \\
	--run-vbls $VBLS 2>&1 | tail -4
ls -la /out/original-boot.avi
"

printf '原版錄影已產出：%s/original-boot.avi\n' "$out_dir"
printf '含原版畫面，不得進版控或 release。\n'
