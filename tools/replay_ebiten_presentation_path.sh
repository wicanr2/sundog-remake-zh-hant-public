#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# 從 title 開始，只用鍵盤驗證呈現預設：F8 走完三種預設（Atari ST 原版風／
# Apple II 原版風／重製版現代風）並繞回起點，每一步畫面都真的變了，而且配色、圖塊
# 與版面三個欄位永遠是同一個預設的三元組。此腳本必須在 sundog-remake-build Docker
# image 內執行；不注入 state、不預製存檔。
# 這只驗證目前 remake prototype，不宣稱 Apple II／Atari ST 呈現 parity。
#
# 需要玩家自備的原版磁碟：沒有磁碟時開場只會停在選碟卡，什麼都按不動。

output_dir="${1:-/tmp/sundog-ebiten-presentation}"
save_path="${2:-/tmp/sundog-ebiten-presentation-save.json}"
library_path="${3:-/tmp/sundog-ebiten-presentation-library.json}"
: "${SUNDOG_ATARI_DISK:?需要 SUNDOG_ATARI_DISK 指向玩家自備的 .st}"
test -f "$SUNDOG_ATARI_DISK"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="${SUNDOG_EBITEN_BINARY:-/tmp/sundog-ebiten-presentation.bin}"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-presentation-runtime}"

if test -e "$output_dir"; then
	test -d "$output_dir"
	test -z "$(find "$output_dir" -mindepth 1 -maxdepth 1 -print -quit)"
else
	mkdir -p "$output_dir"
fi
test ! -e "$save_path"
test ! -e "$library_path"
# 設定檔跟著存檔路徑走；上一輪留下的會在開局套用（例如圖塊集），讓四張圖對不上。
rm -f "${save_path%.json}-settings.json"
mkdir -p "$runtime_dir"

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

digest() {
	sha256sum "$1" | cut -d' ' -f1
}

wait_for_exit() {
	local label="$1"
	local diagnostics="$output_dir/q-process-diagnostics.log"
	local kill_state
	local process_state
	local wait_status

	printf 'phase=%s pid=%s time=%s\n' "${label}-before-q" "$app_pid" "$(date -Is)" >>"$diagnostics"
	for _ in $(seq 1 50); do
		if kill -0 "$app_pid" 2>/dev/null; then
			kill_state=1
		else
			kill_state=0
		fi
		process_state="$(ps -o stat=,pid=,comm= -p "$app_pid" 2>/dev/null | sed 's/^ *//' || true)"
		printf 'phase=%s pid=%s kill-0=%s ps=%s time=%s\n' \
			"${label}-after-q" "$app_pid" "$kill_state" "${process_state:-<gone>}" "$(date -Is)" >>"$diagnostics"
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

DISPLAY="$display_address" XDG_RUNTIME_DIR="$runtime_dir" "$binary_path" -scale 3 \
	-start title -atari-disk "$SUNDOG_ATARI_DISK" \
	-settings "${save_path%.json}-settings.json" -save "$save_path" -library "$library_path" >"$output_dir/app.log" 2>&1 &
app_pid=$!
window="$(find_window)"
sleep 1

# 第一次啟動沒有設定檔，開場會先問呈現；ENTER 取第一個（Atari ST 原版風）。
press Return

# P：登船。地面畫面才畫得到圖塊，所以走到地面再比對，否則換呈現看不出差別。
press p
press g
# 落地動畫（XLANDFX）在地面上跑幾秒，期間只有 Escape／Space 有作用；先跳過它，
# 否則接下來的 F8 會被吃掉，四張圖拍到的是動畫。
press space
capture "$output_dir/01-look-atari-st.png"

# F8 換一整套呈現：配色、圖塊與版面一起換。三軸各自獨立的時代結束於 2026-09-03 的
# 量測——Atari ST 版面下另外兩軸畫不出差別，而其中一組會把重製版地圖浮貼在原版
# 滿版城市上。
press F8
capture "$output_dir/02-look-apple-ii.png"

press F8
capture "$output_dir/03-look-remake-modern.png"

# 第三下繞回第一套：循環走得完，也回得來。
press F8
capture "$output_dir/04-look-atari-st-again.png"

# 回船要走到船那一格（XMOVEONG:0x17 re_enter_ship）：船停在落地格上，
# 往東走開一步再走回來就登船。地面的 Escape 不再回船。
press Right
press Left
press s
quit_app presentation-path
q_fallback_used=0
if ! wait_for_exit presentation-path; then
	q_fallback_used=1
fi

test -s "$save_path"
# 走完一圈之後回到第一套，所以存檔記的是 Atari ST 原版風的三元組——三個欄位必須
# 同時成立，任何一個對不上就是三軸又各走各的了。
grep -Eq '"theme"[[:space:]]*:[[:space:]]*"atari-st-original"' "$save_path"
grep -Eq '"tileset"[[:space:]]*:[[:space:]]*"atari-st-tiles"' "$save_path"
grep -Eq '"layout"[[:space:]]*:[[:space:]]*"atari-st"' "$save_path"
# 設定檔也記住了選擇：下次開機不會再問。
grep -Eq '"presentation"[[:space:]]*:[[:space:]]*"atari-st"' "${save_path%.json}-settings.json"

for image in \
	01-look-atari-st.png \
	02-look-apple-ii.png \
	03-look-remake-modern.png \
	04-look-atari-st-again.png; do
	test -s "$output_dir/$image"
done

# 前三張兩兩不同：每一次按鍵都真的改了畫面，沒有一步是空轉。
digests="$(for image in "$output_dir"/0[1-3]-*.png; do digest "$image"; done | sort -u | wc -l)"
test "$digests" -eq 3
# 第四張回到第一套。訊息窗會多一行「呈現已切換」，所以比的是地面那一塊，不是整張。
test "$(digest "$output_dir/04-look-atari-st-again.png")" != "$(digest "$output_dir/02-look-apple-ii.png")"

sha256sum "$output_dir"/*.png "$save_path"

if test "$q_fallback_used" -ne 0; then
	printf '%s\n' 'Q 未在 5 秒內正常收束；已執行有界 fallback kill，判定失敗。' >&2
	exit 125
fi
