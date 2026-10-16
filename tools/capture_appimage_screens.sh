#!/usr/bin/env bash
set -euo pipefail

# 以封裝好的 AppImage 或目前工作樹建出的 binary 帶原版磁碟逐畫面擷取，
# 供 tools/compare_appimage_to_original.py 與原版一手資料對拍。正式 release 收據用
# AppImage；polish 迭代先用 current binary，避免拿舊封包驗新程式。
#
# 在容器內執行（repo 之外的任何掛載點都可以）：
#   docker run --rm --network none \
#     -v <資料目錄>:/data:ro -v <輸出目錄>:/out \
#     --entrypoint bash sundog-remake-build:20260812 \
#     /data/capture_appimage_screens.sh /data/SunDog.AppImage /data/sundog.st /out [/data/items-save.json]
#
# 資料目錄要放：AppImage、原版 raw .st、本腳本；可選第四個引數是「攜帶物品的存檔」
# ——狀態視窗的圖示列只有身上有東西才會畫，新遊戲是空的。
# 邊界：只寫輸出目錄與 /tmp；不清理任何 docker 資源。

app=${1:?usage: capture_appimage_screens.sh <AppImage> <disk.st> <out-dir> [items-save.json]}
disk=${2:?}
out=${3:?}
items_save=${4:-}
scale=3
settings=/tmp/sundog-atari-ux-settings.json

app_command=("$app")
case "$app" in
	*.AppImage) app_command+=(--appimage-extract-and-run) ;;
esac

export LC_ALL=C.UTF-8
mkdir -p "$out"
command -v import >/dev/null || { echo "需要 ImageMagick 的 import" >&2; exit 2; }
printf '%s\n' '{"schema":1,"theme":"atari-st-original","tileset":"atari-st-tiles","layout":"atari-st"}' >"$settings"

display=":96"
Xvfb "$display" -screen 0 1600x1200x24 -nolisten tcp >/tmp/xvfb.log 2>&1 &
xvfb_pid=$!
export DISPLAY="$display"
trap 'kill "$xvfb_pid" 2>/dev/null || true' EXIT
for _ in $(seq 1 50); do xwininfo -root >/dev/null 2>&1 && break; sleep 0.1; done

find_window() {
	local want=$1 candidate
	for _ in $(seq 1 150); do
		candidate="$(xdotool search --onlyvisible --class ebitengine-application 2>/dev/null | head -n 1 || true)"
		if test -n "$candidate" && xwininfo -id "$candidate" 2>/dev/null | grep -q "Width: $want"; then
			printf '%s\n' "$candidate"
			return 0
		fi
		sleep 0.2
	done
	return 1
}

settle() {
	local window=$1 previous=/tmp/settle-prev.png current=/tmp/settle-cur.png same=0 attempt
	import -window "$window" "$previous"
	for attempt in $(seq 1 40); do
		sleep 1
		import -window "$window" "$current"
		if cmp -s "$previous" "$current"; then
			same=$((same + 1))
			test "$same" -ge 3 && return 0
		else
			same=0
		fi
		cp "$current" "$previous"
	done
	echo "警告：$window 40 秒內沒有靜止" >&2
	return 0
}

# shot <輸出檔> <額外旗標...> -- [按鍵...]
shot() {
	local path=$1
	shift
	local flags=() keys=()
	while test $# -gt 0; do
		if test "$1" = "--"; then
			shift
			keys=("$@")
			break
		fi
		flags+=("$1")
		shift
	done
	# 等上一輪的視窗真的消失。AppImage 是 wrapper 帶子行程，殺錯層的話舊視窗
	# 會留著，find_window 就抓到一張空白圖——所以下面用 setsid ＋ 整組 kill。
	for _ in $(seq 1 60); do
		test -z "$(xdotool search --class ebitengine-application 2>/dev/null || true)" && break
		sleep 0.5
	done
	rm -f /tmp/save.json /tmp/library.json
	setsid "${app_command[@]}" \
		-scale "$scale" -atari-disk "$disk" \
		-settings "$settings" \
		-save /tmp/save.json -library /tmp/library.json \
		"${flags[@]}" >"$out/log-$(basename "$path" .png).txt" 2>&1 &
	local pid=$! window
	window="$(find_window "$((320 * scale))")" || {
		echo "找不到 $path 的視窗" >&2
		kill -TERM -- "-$pid" 2>/dev/null || true
		return 1
	}
	# 無視窗管理器的 Xvfb 不會自動把新視窗設為焦點；Ebiten 失焦時會暫停更新，
	# landing 因而永遠停在某個過場幀。先聚焦再等待 steady frame。
	xdotool windowfocus --sync "$window"
	settle "$window"
	local key
	for key in "${keys[@]}"; do
		# Ebiten 以逐幀 just-pressed 邊緣取樣；瞬間 key 事件可能在兩幀之間完整消失。
		xdotool keydown --clearmodifiers --window "$window" "$key"
		sleep 0.2
		xdotool keyup --clearmodifiers --window "$window" "$key"
		sleep 0.5
	done
	settle "$window"
	import -window "$window" "$path"
	kill -TERM -- "-$pid" 2>/dev/null || true
	wait "$pid" 2>/dev/null || true
	printf '拍好 %s\n' "$path"
}

# 原版七區主控台對應 navigation／駕駛台，不是 shipboard 船艙剖面。
shot "$out/cockpit.png" -start navigation --
# 外觀由設定檔固定，不依賴會隨預設數量改變的 F8 按鍵序列。
shot "$out/ground-atari.png" -start ground --
shot "$out/tactical.png" -start tactical --
shot "$out/engineering.png" -start engineering --
# 交易所：原版的對照是走進城市槽位 2 開出的 `Drahew Exch.`／`PRICE ON GOODS` 兩張，
# 收據見 docs/atari-st/ux-scene-ledger.md 第七輪。
shot "$out/trade.png" -start trade --
# 星系圖（原版是軌道上點 SUB-LIGHT 開的那張）：Down 是 ActionNextTargetPlanet，
# 它把 NavigationView 切成 system。
shot "$out/navmap-system.png" -start navigation -- Down
# Uniteller：原版的對照是 `Jondd Bank`（框寬 170、三條帶各高 13），
# 收據見 docs/atari-st/ux-scene-ledger.md 第八輪。
shot "$out/uniteller.png" -start uniteller --
# 電車站：原版的對照是走進城市槽位 0 開出的 `Tube Station`（標題列 + 置中兩行 +
# pick_location 的框），收據見 docs/atari-st/ux-scene-ledger.md 第九輪。
shot "$out/tube-station.png" -start tube-station --
if test -n "$items_save"; then
	# -load 與 -start 不能同用（-start 的重播序列從 title 起步，載入後的畫面不是
	# title 就會失敗），所以載入後用 h 鍵開狀態視窗。
	cp "$items_save" /tmp/items-save.json
	shot "$out/status-items.png" -load -save /tmp/items-save.json -- h
	printf '%s\n' "$(basename "$items_save")" >"$out/status-items.fixture"
else
	rm -f "$out/status-items.fixture"
	shot "$out/status-items.png" -start status --
fi
identify "$out"/*.png
