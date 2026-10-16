#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# 必須在專案 Docker toolchain 內、repository root 執行。此重播從 title 開始，
# 只使用實際滑鼠點擊：可見的「開始遊玩」文字、已由 Apple IIe oracle 證實的
# Sub-C Engines 船艙目標，以及重製版地面面板中可見的「購買燃料」。它不注入
# game state、地圖座標、道具或強制勝利。

output_dir="${1:-/tmp/sundog-ebiten-title-engineering-mouse}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="/tmp/sundog-ebiten-title-engineering-mouse.bin"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-runtime}"

mkdir -p "$output_dir" "$runtime_dir"
# v.1.0.1 起沒有原版磁碟就停在選碟卡，進不了遊戲——沒有這一行，這支腳本會把按鍵打進
# 選碟卡的路徑欄，然後在最後的斷言那裡才失敗。
: "${SUNDOG_ATARI_DISK:?需要 SUNDOG_ATARI_DISK 指向玩家自備的 .st}"
test -f "$SUNDOG_ATARI_DISK"

go build -buildvcs=false -o "$binary_path" ./cmd/sundog-ebiten

Xvfb "$display_address" -screen 0 1280x800x24 -nolisten tcp >/tmp/sundog-ebiten-title-engineering-xvfb.log 2>&1 &
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

DISPLAY="$display_address" XDG_RUNTIME_DIR="$runtime_dir" "$binary_path" -scale 3 -atari-disk "$SUNDOG_ATARI_DISK" -settings /tmp/sundog-ebiten-replay-settings.json -start title >/tmp/sundog-ebiten-title-engineering.log 2>&1 &
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

# v.1.0.1 起設定檔是新的就會先問一次「呈現方式」，v.1.0.2 起標題上還有一張告示。
# 兩張都要先關掉，不然下面第一個點擊會打在卡片上。
DISPLAY="$display_address" xdotool key --clearmodifiers --window "$window" Return
sleep 1
DISPLAY="$display_address" xdotool key --clearmodifiers --window "$window" space
sleep 1

DISPLAY="$display_address" import -window "$window" "$output_dir/01-title.png"

# 視窗為 960×600，邏輯畫布為 320×200；(300,222) 對應 title 中可見的
# 「開始遊玩」列（logical 約 100,74）。
DISPLAY="$display_address" xdotool mousemove --sync --window "$window" 300 222
DISPLAY="$display_address" xdotool click --delay 100 1
sleep 1

# (300,282) 對應 logical 約 (100,94)，落在已證據化的 Sub-C Engines 目標。
DISPLAY="$display_address" xdotool mousemove --sync --window "$window" 300 282
DISPLAY="$display_address" xdotool click --delay 100 1
sleep 1
DISPLAY="$display_address" import -window "$window" "$output_dir/02-engineering.png"

# 工程艙的右鍵語意為返回船艦，接著以既有底部地面入口進入地面畫面。
DISPLAY="$display_address" xdotool mousemove --sync --window "$window" 300 282
DISPLAY="$display_address" xdotool click --delay 100 3
sleep 1

# (90,510) 對應 shipboard footer 的第一個地面入口（logical 約 30,170）。
DISPLAY="$display_address" xdotool mousemove --sync --window "$window" 90 510
DISPLAY="$display_address" xdotool click --delay 100 1
sleep 1

# (150,240) 對應地面面板中可見的「購買燃料」列（logical 約 50,80）。
DISPLAY="$display_address" xdotool mousemove --sync --window "$window" 150 240
DISPLAY="$display_address" xdotool click --delay 100 1
sleep 1
DISPLAY="$display_address" import -window "$window" "$output_dir/03-refuel.png"

sha256sum "$output_dir/01-title.png" "$output_dir/02-engineering.png" "$output_dir/03-refuel.png"
