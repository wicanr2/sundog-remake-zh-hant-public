#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# 從 title 開始，以正常鍵盤輸入完成一次系統內航行：選定航線、逐刻推進、抵達、存檔。
# 此腳本必須在 sundog-remake-build Docker image 內執行；不使用 start tactical、
# debug、狀態注入、座標／物品注入或預製存檔。這只驗證目前 remake prototype，
# 不宣稱 Apple II／Atari ST 規則 parity。

output_dir="${1:-/tmp/sundog-ebiten-flight}"
save_path="${2:-/tmp/sundog-ebiten-flight-save.json}"
library_path="${3:-/tmp/sundog-ebiten-flight-library.json}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="${SUNDOG_EBITEN_BINARY:-/tmp/sundog-ebiten-flight.bin}"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-flight-runtime}"

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

	printf 'phase=%s pid=%s time=%s\n' "${label}-before-q" "$app_pid" "$(date -Is)" >>"$diagnostics"
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
start_app "$output_dir/app.log"
capture "$output_dir/01-title.png"

# Return：確認呈現方式那張卡（設定檔是新的，所以每次都會先問一次）；
# Space：關掉標題告示。
press Return
press space
# P：登船；N：開啟導航。預設目標與出發地在同一顆行星，極座標一樣，航程只有一刻；
# 按一次 Down 換到同一星系的另一顆行星，才會有真正的航程。Enter 確認航線之後不是
# 直接抵達，而是進入系統內航行。
press p
press n
press Down
press Return
capture "$output_dir/02-flight-start.png"

# Enter 每次推進一刻。極座標插值的長度由兩顆行星的位置決定，不由這個腳本決定；
# 這裡推進 12 刻，取一張航行中的畫面。
for _ in $(seq 1 12); do
	press Return
done
capture "$output_dir/03-flight-under-way.png"

# E：把剩下的航程一次飛完，正常抵達並回到船艦畫面。
press e
capture "$output_dir/04-arrived-shipboard.png"
press s
quit_app flight-path
q_fallback_used=0
if ! wait_for_exit flight-path; then
	q_fallback_used=1
fi

test -s "$save_path"
grep -Eq '"screen"[[:space:]]*:[[:space:]]*"shipboard"' "$save_path"
grep -Eq '"planetId"[[:space:]]*:[[:space:]]*"heavy"' "$save_path"
grep -Eq '"cityId"[[:space:]]*:[[:space:]]*"lightning"' "$save_path"
# 抵達之後船艦就停在目標行星的極座標上：赫維的半徑 30、角度 10，兩者都直接來自磁碟表。
grep -Eq '"flightRadius"[[:space:]]*:[[:space:]]*30' "$save_path"
grep -Eq '"flightAngle"[[:space:]]*:[[:space:]]*10' "$save_path"

for image in 01-title.png 02-flight-start.png 03-flight-under-way.png 04-arrived-shipboard.png; do
	test -s "$output_dir/$image"
done
sha256sum "$output_dir"/*.png "$save_path"

if test "$q_fallback_used" -ne 0; then
	printf '%s\n' 'Q 未在 5 秒內正常收束；已執行有界 fallback kill，判定失敗。' >&2
	exit 125
fi
