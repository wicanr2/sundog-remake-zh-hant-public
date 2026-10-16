#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# UI-007：以正常鍵盤從戰術回火進入受損工程艙，修理選定系統、查看 Equipment、存檔並退出。
# 這只驗證 remake-owned prototype；未證明 Apple II／Atari ST 的戰鬥、損傷、分流器、設備功能、UI 或存檔 parity。

output_dir="${1:-/tmp/sundog-ebiten-engineering-equipment}"
save_path="${2:-/tmp/sundog-ebiten-engineering-equipment-save.json}"
library_path="${3:-/tmp/sundog-ebiten-engineering-equipment-library.json}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="${SUNDOG_EBITEN_BINARY:-/tmp/sundog-ebiten-engineering-equipment.bin}"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-engineering-equipment-runtime}"

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
export GOCACHE="${SUNDOG_GOCACHE:-/tmp/sundog-ebiten-engineering-equipment-gocache}"
export XDG_RUNTIME_DIR="$runtime_dir"
mkdir -p "$GOCACHE"

printf '%s\n' 'UI-007：建置 remake-owned 工程艙／Equipment prototype。'
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
		if DISPLAY="$display_address" xwininfo -root >/dev/null 2>&1; then return 0; fi
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
			printf '%s\n' "$candidate"; return 0
		fi
		candidate="$(DISPLAY="$display_address" xwininfo -root -tree 2>/dev/null | awk '/960x600/ {print $1; exit}')"
		if test -n "$candidate" && DISPLAY="$display_address" xwininfo -id "$candidate" 2>/dev/null | grep -q 'Width: 960'; then
			printf '%s\n' "$candidate"; return 0
		fi
		sleep 0.2
	done
	return 1
}

start_app() {
	DISPLAY="$display_address" XDG_RUNTIME_DIR="$runtime_dir" "$binary_path" -scale 3 \
		-start title -atari-disk "$SUNDOG_ATARI_DISK" -settings "${save_path%.json}-settings.json" -save "$save_path" -library "$library_path" >"$output_dir/app.log" 2>&1 &
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

quit_app() {
	local q_window q_output q_status
	if q_window="$(find_window)"; then
		if q_output="$(DISPLAY="$display_address" xdotool key --clearmodifiers --window "$q_window" q 2>&1)"; then q_status=0; else q_status=$?; fi
	else
		q_status=1
		q_output='Q 前找不到 960×600 Ebiten 視窗'
	fi
	printf 'phase=engineering-equipment q-send-status=%s window=%s\n' "$q_status" "${q_window:-<none>}" >>"$output_dir/q-process-diagnostics.log"
	if test -n "$q_output"; then printf 'Q 診斷：%s\n' "$q_output" >>"$output_dir/q-process-diagnostics.log"; fi
}

wait_for_exit() {
	local diagnostics="$output_dir/q-process-diagnostics.log"
	local kill_state
	local process_state
	local wait_status

	printf 'phase=engineering-equipment-before-q pid=%s time=%s\n' "$app_pid" "$(date -Is)" >>"$diagnostics"
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
		printf 'phase=engineering-equipment-after-q pid=%s kill-0=%s ps=%s time=%s\n' \
			"$app_pid" "$kill_state" "${process_state:-<gone>}" "$(date -Is)" >>"$diagnostics"
		if test "$kill_state" -eq 0 || printf '%s\n' "$process_state" | grep -q '^Z'; then
			if wait "$app_pid"; then
				wait_status=0
			else
				wait_status=$?
			fi
			printf 'phase=engineering-equipment result=normal-q wait-status=%s time=%s\n' \
				"$wait_status" "$(date -Is)" >>"$diagnostics"
			app_pid=""
			return "$wait_status"
		fi
		sleep 0.1
	done

	printf 'phase=engineering-equipment result=q-timeout action=kill pid=%s time=%s\n' \
		"$app_pid" "$(date -Is)" >>"$diagnostics"
	kill "$app_pid" 2>/dev/null || true
	if wait "$app_pid"; then
		wait_status=0
	else
		wait_status=$?
	fi
	printf 'phase=engineering-equipment result=fallback-kill wait-status=%s time=%s\n' \
		"$wait_status" "$(date -Is)" >>"$diagnostics"
	app_pid=""
	printf 'Q 未在 5 秒內正常收束；已執行有界 fallback kill，UI-007 判定失敗。\n' >&2
	return 125
}

wait_for_display
start_app
capture "$output_dir/01-title.png"

# Return：確認呈現方式那張卡（設定檔是新的，所以每次都會先問一次）；
# Space：關掉標題告示。
press Return
press space
# 空艙不會招來海盜（`PodDrawsEncounter`，update_movement 的第四道閘門），沒有敵艦
# 就沒有回火、沒有損傷，工程艙也就沒有東西可修——分流器不會被收走，最後在
# shunts 那條斷言才失敗。所以先下地面裝一批補給再開戰術顯示器。
# G：進地面；Escape：跳過落地；P：在起始倉庫裝補給；Right／Left：走開一步再走回
# 船那一格登船。
press p
press g
press Escape
press p
press Right
press Left
# X→F：正常進入戰術並開火。敵艦有裝填倒數，第一發不會馬上回來，所以之後再以
# M 推進到敵艦真的打中為止。**需要幾回合是由這一局的敵艦型號、方位與距離決定的，
# 不是固定值**——早先寫的 16 回合對後來的生成序列已經不夠（跑出殲滅者時 16 回合
# 一發都沒挨到，工程艙因此沒東西可修）。40 回合是留了餘裕的上界；這條收據對
# 生成序列仍然敏感，與 UI-003 是同一類脆弱。
press x
press f
for _ in $(seq 1 40); do
	press m
done
capture "$output_dir/02-tactical-retaliation.png"
# 戰術畫面的第一個 Escape 是「停止滑鼠轉向」（do_guns 的迴圈在按鍵時退出），
# 不是離開；要第二個 Escape 才回船艦。舊腳本只按一次，所以之後每一個鍵都打在
# 戰術畫面上、什麼也沒發生。
press Escape
press Escape
press e
capture "$output_dir/03-engineering-damaged.png"

# E：修理目前選定且受損的系統，消耗唯一分流器；I：只讀 Equipment 索引。
press e
capture "$output_dir/04-engineering-repaired.png"
press i
capture "$output_dir/05-equipment-index.png"
press Escape
press Escape
capture "$output_dir/06-returned-shipboard.png"
press s
quit_app
wait_for_exit

test -s "$save_path"
# 新遊戲帶 5 個分流器（XSTARTUP:0x32 set_up_lockers 的實際內容），修理花掉一個。
grep -Eq '"shunts"[[:space:]]*:[[:space:]]*4' "$save_path"
grep -Eq '"screen"[[:space:]]*:[[:space:]]*"shipboard"' "$save_path"
for image in 01-title.png 02-tactical-retaliation.png 03-engineering-damaged.png \
	04-engineering-repaired.png 05-equipment-index.png 06-returned-shipboard.png; do
	test -s "$output_dir/$image"
done
sha256sum "$output_dir"/*.png "$save_path"
