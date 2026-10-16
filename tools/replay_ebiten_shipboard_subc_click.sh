#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# 必須在 project Docker toolchain 內、repository root 執行。
# 這是正常滑鼠路徑 smoke：不注入 state、座標、不發道具，也不強制勝利。
# 輸出圖片只放到 /tmp 或呼叫端指定的可寫目錄。

output_dir="${1:-/tmp/sundog-ebiten-subc-click}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="/tmp/sundog-ebiten-subc-click.bin"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-runtime}"

mkdir -p "$output_dir" "$runtime_dir"
# v.1.0.1 起沒有原版磁碟就停在選碟卡，進不了遊戲——沒有這一行，這支腳本會把按鍵打進
# 選碟卡的路徑欄，然後在最後的斷言那裡才失敗。
: "${SUNDOG_ATARI_DISK:?需要 SUNDOG_ATARI_DISK 指向玩家自備的 .st}"
test -f "$SUNDOG_ATARI_DISK"

go build -o "$binary_path" ./cmd/sundog-ebiten

Xvfb "$display_address" -screen 0 1280x800x24 -nolisten tcp >/tmp/sundog-ebiten-subc-xvfb.log 2>&1 &
xvfb_pid=$!
app_pid=""
cleanup() {
	if test -n "$app_pid"; then
		kill "$app_pid" 2>/dev/null || true
		wait "$app_pid" 2>/dev/null || true
	fi
	kill "$xvfb_pid" 2>/dev/null || true
}
trap cleanup EXIT

DISPLAY="$display_address" XDG_RUNTIME_DIR="$runtime_dir" "$binary_path" -scale 3 -atari-disk "$SUNDOG_ATARI_DISK" -settings /tmp/sundog-ebiten-replay-settings.json -start shipboard >/tmp/sundog-ebiten-subc.log 2>&1 &
app_pid=$!

window=""
for _ in $(seq 1 100); do
	for candidate in $(DISPLAY="$display_address" xdotool search --onlyvisible --name "" 2>/dev/null || true); do
		if DISPLAY="$display_address" xwininfo -id "$candidate" 2>/dev/null | grep -q "Width: 960"; then
			window="$candidate"
			break
		fi
	done
	if test -n "$window"; then
		break
	fi
	sleep 0.2
done
test -n "$window"
sleep 1

# 設定檔是新的就會先問一次「呈現方式」，要先關掉才看得到船艙。
DISPLAY="$display_address" xdotool key --clearmodifiers --window "$window" Return
sleep 1

DISPLAY="$display_address" import -window "$window" "$output_dir/before.png"

# 視窗為 960×600，logical 320×200；(300,282) 對應 logical 約 (100,94)，
# 落在 evidence-backed `Sub-C Engines` panel target。
DISPLAY="$display_address" xdotool mousemove --sync --window "$window" 300 282
DISPLAY="$display_address" xdotool click --delay 100 1
sleep 1
DISPLAY="$display_address" import -window "$window" "$output_dir/after.png"

sha256sum "$output_dir/before.png" "$output_dir/after.png"
