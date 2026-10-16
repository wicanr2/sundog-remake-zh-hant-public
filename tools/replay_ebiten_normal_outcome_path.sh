#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# UI-001：從 title 開始，以正常鍵盤輸入交付任務、查看成功計分、存檔，再由第二個
# title 程序透過正常載入路徑讀回。此腳本必須在 sundog-remake-build
# Docker image 內執行；不使用 start outcome、debug、狀態注入或預製存檔。

output_dir="${1:-/tmp/sundog-ebiten-normal-outcome}"
save_path="${2:-/tmp/sundog-ebiten-normal-outcome-save.json}"
library_path="${3:-/tmp/sundog-ebiten-normal-outcome-library.json}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="${SUNDOG_EBITEN_BINARY:-/tmp/sundog-ebiten-normal-outcome.bin}"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-normal-runtime}"

# 驗收必須從全新的 output／save／library 開始。可接受呼叫端以 `mktemp -d`
# 建立的空 output 目錄；但拒絕其中已有檔案，避免覆寫舊證據或混入預製產物。
if test -e "$output_dir"; then
	test -d "$output_dir"
	test -z "$(find "$output_dir" -mindepth 1 -maxdepth 1 -print -quit)"
else
	mkdir -p "$output_dir"
fi
test ! -e "$save_path"
test ! -e "$library_path"
mkdir -p "$runtime_dir"

# v.1.0.1 起沒有原版磁碟就停在選碟卡，進不了遊戲——沒有這一行，這支腳本會把按鍵打進
# 選碟卡的路徑欄，然後在最後的斷言那裡才失敗。
: "${SUNDOG_ATARI_DISK:?需要 SUNDOG_ATARI_DISK 指向玩家自備的 .st}"
test -f "$SUNDOG_ATARI_DISK"

export PATH="/usr/local/go/bin:/go/bin:$PATH"
export GOCACHE="${SUNDOG_GOCACHE:-/tmp/sundog-ebiten-gocache}"
export XDG_RUNTIME_DIR="$runtime_dir"
mkdir -p "$GOCACHE"

go build -buildvcs=false -o "$binary_path" ./cmd/sundog-ebiten

Xvfb "$display_address" -screen 0 1280x800x24 -nolisten tcp >"$output_dir/xvfb.log" 2>&1 &
xvfb_pid=$!
app_pid=""
window=""

cleanup() {
	if test -n "$app_pid" && kill -0 "$app_pid" 2>/dev/null; then
		kill "$app_pid" 2>/dev/null || true
		wait "$app_pid" 2>/dev/null || true
	fi
	if kill -0 "$xvfb_pid" 2>/dev/null; then
		kill "$xvfb_pid" 2>/dev/null || true
		wait "$xvfb_pid" 2>/dev/null || true
	fi
}
trap cleanup EXIT

wait_for_display() {
	for _ in $(seq 1 50); do
		if DISPLAY="$display_address" xwininfo -root >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.1
	done
	printf 'Xvfb 未在 5 秒內就緒：%s\n' "$display_address" >&2
	return 1
}

find_window() {
	local candidate
	for _ in $(seq 1 120); do
		candidate="$(DISPLAY="$display_address" xdotool search --onlyvisible --class ebitengine-application 2>/dev/null | head -n 1 || true)"
		if test -n "$candidate" && DISPLAY="$display_address" xwininfo -id "$candidate" 2>/dev/null | grep -q 'Width: 960'; then
			printf '%s\n' "$candidate"
			return 0
		fi
		candidate="$(DISPLAY="$display_address" xwininfo -root -tree 2>/dev/null | awk '/960x600/ {print $1; exit}')"
		if test -n "$candidate" && DISPLAY="$display_address" xwininfo -id "$candidate" 2>/dev/null | grep -q 'Width: 960'; then
			printf '%s\n' "$candidate"
			return 0
		fi
		sleep 0.2
	done
	return 1
}

start_app() {
	local log_path="$1"
	DISPLAY="$display_address" XDG_RUNTIME_DIR="$runtime_dir" "$binary_path" -scale 3 \
		-start title -atari-disk "$SUNDOG_ATARI_DISK" -settings "${save_path%.json}-settings.json" -save "$save_path" -library "$library_path" >"$log_path" 2>&1 &
	app_pid=$!
	window="$(find_window)"
	sleep 1
}

press() {
	window="$(find_window)"
	DISPLAY="$display_address" xdotool key --clearmodifiers --window "$window" "$1"
	sleep 0.7
}

capture() {
	local path="$1"
	window="$(find_window)"
	DISPLAY="$display_address" import -window "$window" "$path"
	test -s "$path"
}

wait_for_exit() {
	local label="$1"
	local diagnostics="$output_dir/q-process-diagnostics.log"
	local kill_state
	local process_state
	local wait_status

	printf 'phase=%s pid=%s time=%s\n' "$label-before-q" "$app_pid" "$(date -Is)" >>"$diagnostics"
	for _ in $(seq 1 50); do
		if kill -0 "$app_pid" 2>/dev/null; then
			kill_state=1
		else
			kill_state=0
		fi
		# pipefail is on: once the app exits, `ps` returns 1 and the whole
		# pipeline fails, which under errexit aborts the script mid-loop — a run
		# that was about to succeed then reports failure. The process going away
		# is exactly what this loop is waiting for, so it must not be an error.
		process_state="$(ps -o stat=,pid=,comm= -p "$app_pid" 2>/dev/null | sed 's/^ *//' || true)"
		printf 'phase=%s pid=%s kill-0=%s ps=%s time=%s\n' \
			"$label-after-q" "$app_pid" "$kill_state" "${process_state:-<gone>}" "$(date -Is)" >>"$diagnostics"
		if test "$kill_state" -eq 0 || printf '%s\n' "$process_state" | grep -q '^Z'; then
			if wait "$app_pid"; then
				wait_status=0
			else
				wait_status=$?
			fi
			printf 'phase=%s result=normal-q wait-status=%s time=%s\n' \
				"$label" "$wait_status" "$(date -Is)" >>"$diagnostics"
			app_pid=""
			return "$wait_status"
		fi
		sleep 0.1
	done

	printf 'phase=%s result=q-timeout action=kill pid=%s time=%s\n' \
		"$label" "$app_pid" "$(date -Is)" >>"$diagnostics"
	kill "$app_pid" 2>/dev/null || true
	if wait "$app_pid"; then
		wait_status=0
	else
		wait_status=$?
	fi
	printf 'phase=%s result=fallback-kill wait-status=%s time=%s\n' \
		"$label" "$wait_status" "$(date -Is)" >>"$diagnostics"
	app_pid=""
	return 125
}

quit_app() {
	local label="$1"
	local diagnostics="$output_dir/q-process-diagnostics.log"
	local q_status
	local q_output
	local q_window

	if q_window="$(find_window)"; then
		if q_output="$(DISPLAY="$display_address" xdotool key --clearmodifiers --window "$q_window" q 2>&1)"; then
			q_status=0
		else
			q_status=$?
		fi
	else
		q_status=$?
		q_output="find_window failed before q"
	fi

	printf 'phase=%s q-send-status=%s window=%s time=%s\n' \
		"$label" "$q_status" "${q_window:-<none>}" "$(date -Is)" >>"$diagnostics"
	if test -n "$q_output"; then
		printf 'phase=%s q-send-stderr=%s\n' "$label" "$q_output" >>"$diagnostics"
	fi
	return 0
}

wait_for_display
start_app "$output_dir/first-app.log"
capture "$output_dir/01-title.png"

# Return：確認呈現方式那張卡（設定檔是新的，所以每次都會先問一次）。
press Return
# 現行標題先顯示告示；第一個鍵只關閉告示，不能同時當選單動作。
# Space：關閉告示；P：登船；G：進入地面；P/K：在起始倉庫裝載補給與冷凍艙貨物。
press space
press p
press g
# XLANDFX 吃掉其他按鍵；Escape 明確跳到 landing idle，之後 P／K 才是地面動作。
press Escape
press p
press k
# R：買燃料。D6 之後新遊戲的燃料照原版是 0，而導航到 Banville 要 4 單位——
# 沒有這一行，導航畫面開得起來但飛不動，後面每一步都跟著空轉。舊腳本是燃料
# 還預設 100 的年代寫的。
press r
# 回船要走到船那一格：船停在落地格上，所以往東走開一步再走回來就登船。
press Right
press Left

# N：導航；G：以預設目標（任務目的地 Banville）起飛。起飛之後是飛行畫面，
# **要按 E 把剩下的航程一次飛完**——舊腳本在這裡再按一次 G，那個鍵打在飛行畫面上
# 沒有作用，所以船一直沒到，後面每一步都跟著空轉。
press n
press g
press e
# E 一次飛完之後先播抵達的落地動畫，Escape 跳過它會回到船艦（訊息「落地。」）；
# 要下城市還是得再按一次 G，那一次也有自己的落地動畫要跳過，然後才輪到 V 交付。
press Escape
press g
press Escape
press v
capture "$output_dir/02-score-success.png"

# Escape：從成功計分正常返回船艦；S：正常存檔。
press Escape
press s
capture "$output_dir/03-after-save.png"
quit_app first-app
q_fallback_used=0
if ! wait_for_exit first-app; then
	q_fallback_used=1
fi

test -s "$save_path"
grep -Eq '"missionComplete"[[:space:]]*:[[:space:]]*true' "$save_path"
grep -Eq '"outcomeKey"[[:space:]]*:[[:space:]]*"ui\.outcome\.success"' "$save_path"
grep -Eq '"screen"[[:space:]]*:[[:space:]]*"shipboard"' "$save_path"

start_app "$output_dir/second-app.log"
capture "$output_dir/04-reload-title.png"
# 第二個程序同樣先關閉標題告示，L 才是載入動作。
press space
press l
capture "$output_dir/05-loaded-save.png"
quit_app second-app
if ! wait_for_exit second-app; then
	q_fallback_used=1
fi

for image in 01-title.png 02-score-success.png 03-after-save.png 04-reload-title.png 05-loaded-save.png; do
	test -s "$output_dir/$image"
done
sha256sum "$output_dir/01-title.png" "$output_dir/02-score-success.png" \
	"$output_dir/03-after-save.png" "$output_dir/04-reload-title.png" \
	"$output_dir/05-loaded-save.png" "$save_path"

if test "$q_fallback_used" -ne 0; then
	printf '%s\n' 'Q 未在 5 秒內正常收束；已執行有界 fallback kill，UI-001 判定失敗。' >&2
	exit 125
fi
