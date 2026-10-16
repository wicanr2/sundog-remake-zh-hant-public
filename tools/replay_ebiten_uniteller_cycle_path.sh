#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# UI-006：從 title 以正常鍵盤進入既有 Uniteller，完成存入／提領／跨星系轉帳循環，返回船艦、存檔。
# 此腳本必須在 sundog-remake-build Docker image 內執行；不使用 start uniteller、debug、
# state／座標注入、預製存檔或 direct-entry。這只驗證 remake-owned prototype，
# 不宣稱 Apple II／Atari ST 原版銀行、轉帳、經濟、地圖、輸入或 UI parity。

output_dir="${1:-/tmp/sundog-ebiten-uniteller-cycle}"
save_path="${2:-/tmp/sundog-ebiten-uniteller-cycle-save.json}"
library_path="${3:-/tmp/sundog-ebiten-uniteller-cycle-library.json}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="${SUNDOG_EBITEN_BINARY:-/tmp/sundog-ebiten-uniteller-cycle.bin}"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-uniteller-runtime}"

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
export GOCACHE="${SUNDOG_GOCACHE:-/tmp/sundog-ebiten-uniteller-gocache}"
export XDG_RUNTIME_DIR="$runtime_dir"
mkdir -p "$GOCACHE"

printf '%s\n' 'UI-006：建置 remake-owned Uniteller 經濟循環 prototype。'
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
		-atari-disk "$SUNDOG_ATARI_DISK" -settings "${save_path%.json}-settings.json" -save "$save_path" -library "$library_path" >"$log_path" 2>&1 &
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
	printf '畫面=%s\n' "$path"
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
		q_output='Q 前找不到 960×600 Ebiten 視窗'
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
# P：正常進入船艦；G：正常離船；U：在既有 Uniteller 設施開啟銀行畫面。
press p
press g
# Escape：跳過落地動畫。沒有這一行，接下來那個鍵會被落地畫面吃掉。
press Escape
press u
capture "$output_dir/02-uniteller-initial.png"

# Uniteller 已經改成原版的鍵盤面板（`move_cash`）：D／W／T 只是開金額頁，
# A 是「全部」，而**成交點是離開面板**——按 Escape 才真的動錢。舊腳本把 D 當成
# 一鍵存入，所以錢從頭到尾沒動過。
# 開局現金由 XSTARTUP:0x30 的 random() mod 700 + 200 決定（200–899），每一局不同。
press d
press a
press Escape
capture "$output_dir/03-uniteller-deposit.png"

# W：提領全部。本地銀行原本就有一筆錢（世界表帶來的），所以提完現金會遠大於開局值。
press w
press a
press Escape
capture "$output_dir/04-uniteller-withdraw.png"

# D：再全部存回去，現金歸零。T：轉帳先要挑目的地（pick_location，1–6 或 O）；
# 這張收據只走到挑選畫面，再用 Escape 退回櫃台——同星系轉帳本來就是 no-op。
press d
press a
press Escape
press t
capture "$output_dir/05-uniteller-transfer.png"
press Escape

press Escape
# 回船要走到船那一格：船停在落地格上，往東走開一步再走回來就登船。
press Right
press Left
capture "$output_dir/06-returned-shipboard.png"
press s
quit_app uniteller-cycle
q_fallback_used=0
if ! wait_for_exit uniteller-cycle; then
	q_fallback_used=1
fi

test -s "$save_path"
# 本地銀行不再從 0 開始——世界表接進來之後 Jondd 本來就有一筆錢，所以舊的
# 「餘額落在 200–899」已經不成立。現在驗的是循環本身：全部存回去之後現金歸零，
# 而「本地銀行」就是所在星系那一格的餘額。
python3 - "$save_path" <<'PYEOF'
import json, sys
state = json.load(open(sys.argv[1]))["state"]
assert state["cash"] == 0, f"全部存回去之後現金不是 0：{state['cash']}"
assert state["screen"] == "shipboard", f"最後不在船上：{state['screen']}"
balances = state["bankBalances"]
assert state["bank"] > 0, "本地銀行是空的"
assert state["bank"] == balances["jondd"], f"本地銀行 {state['bank']} 與 jondd {balances['jondd']} 不一致"
PYEOF

for image in 01-title.png 02-uniteller-initial.png 03-uniteller-deposit.png \
	04-uniteller-withdraw.png 05-uniteller-transfer.png 06-returned-shipboard.png; do
	test -s "$output_dir/$image"
done
sha256sum "$output_dir/01-title.png" "$output_dir/02-uniteller-initial.png" \
	"$output_dir/03-uniteller-deposit.png" "$output_dir/04-uniteller-withdraw.png" \
	"$output_dir/05-uniteller-transfer.png" "$output_dir/06-returned-shipboard.png" "$save_path"

if test "$q_fallback_used" -ne 0; then
	printf '%s\n' 'Q 未在 5 秒內正常收束；已執行有界 fallback kill，UI-006 判定失敗。' >&2
	exit 125
fi
printf '%s\n' 'UI-006 通過：僅驗證 remake-owned prototype，且 Q 正常收束。'
