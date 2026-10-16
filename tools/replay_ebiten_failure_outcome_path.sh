#!/usr/bin/env bash
set -euo pipefail

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# UI-008：從 title 以正常鍵盤觸發失敗 outcome、保存終局狀態並退出。
# 這只驗證 remake-owned prototype；不證明 Apple II／Atari ST 的敵軍、傷害、
# 失敗、UI 或存檔 parity。

if test "$#" -gt 3; then
	printf '%s\n' '用法：replay_ebiten_failure_outcome_path.sh [輸出目錄] [save JSON] [library JSON]' >&2
	exit 2
fi

output_dir="${1:-/tmp/sundog-ebiten-failure-outcome}"
save_path="${2:-/tmp/sundog-ebiten-failure-outcome-save.json}"
library_path="${3:-/tmp/sundog-ebiten-failure-outcome-library.json}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="${SUNDOG_EBITEN_BINARY:-/tmp/sundog-ebiten-failure-outcome.bin}"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-failure-outcome-runtime}"

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
export GOCACHE="${SUNDOG_GOCACHE:-/tmp/sundog-ebiten-failure-outcome-gocache}"
export XDG_RUNTIME_DIR="$runtime_dir"
mkdir -p "$GOCACHE"

printf '%s\n' 'UI-008：建置 remake-owned 失敗 outcome prototype。'
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
	printf '%s\n' '找不到 960×600 Ebiten 子視窗。' >&2
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

wait_for_exit() {
	local label="$1"
	local diagnostics="$output_dir/q-process-diagnostics.log"
	local kill_state
	local process_state
	local wait_status

	printf 'phase=%s-before-q pid=%s time=%s\n' "$label" "$app_pid" "$(date -Is)" >>"$diagnostics"
	for _ in $(seq 1 50); do
		if kill -0 "$app_pid" 2>/dev/null; then kill_state=1; else kill_state=0; fi
		# pipefail is on: once the app exits, `ps` returns 1 and the whole
		# pipeline fails, which under errexit aborts the script mid-loop — a run
		# that was about to succeed then reports failure. The process going away
		# is exactly what this loop is waiting for, so it must not be an error.
		process_state="$(ps -o stat=,pid=,comm= -p "$app_pid" 2>/dev/null | sed 's/^ *//' || true)"
		printf 'phase=%s-after-q pid=%s kill-0=%s ps=%s time=%s\n' \
			"$label" "$app_pid" "$kill_state" "${process_state:-<gone>}" "$(date -Is)" >>"$diagnostics"
		if test "$kill_state" -eq 0 || printf '%s\n' "$process_state" | grep -q '^Z'; then
			if wait "$app_pid"; then wait_status=0; else wait_status=$?; fi
			printf 'phase=%s result=normal-q wait-status=%s time=%s\n' "$label" "$wait_status" "$(date -Is)" >>"$diagnostics"
			app_pid=""
			return "$wait_status"
		fi
		# errexit is on, and a `wait` interrupted by the child's own SIGCHLD can
		# return non-zero here. Without this guard the loop aborts the whole
		# script on its first pass and a run that met every assertion reports
		# failure.
		sleep 0.1 || true
	done

	printf 'phase=%s result=q-timeout action=kill pid=%s time=%s\n' "$label" "$app_pid" "$(date -Is)" >>"$diagnostics"
	kill "$app_pid" 2>/dev/null || true
	if wait "$app_pid"; then wait_status=0; else wait_status=$?; fi
	printf 'phase=%s result=fallback-kill wait-status=%s time=%s\n' "$label" "$wait_status" "$(date -Is)" >>"$diagnostics"
	app_pid=""
	printf '%s\n' 'Q 未在 5 秒內正常收束；已執行有界 fallback kill，UI-008 判定失敗。' >&2
	return 125
}

quit_app() {
	local q_window q_output q_status
	# `import` 的 X request 完成後再送 Q，避免 capture 與下一個輸入在同一個
	# X event turn 競爭；這是 harness 時序，不改 runtime 的按鍵處理。
	sleep 0.7
	if q_window="$(find_window)"; then
		if q_output="$(DISPLAY="$display_address" xdotool key --clearmodifiers --window "$q_window" q 2>&1)"; then q_status=0; else q_status=$?; fi
	else
		q_status=1
		q_output='Q 前找不到 960×600 Ebiten 視窗'
	fi
	printf 'phase=failure-outcome q-send-status=%s window=%s time=%s\n' "$q_status" "${q_window:-<none>}" "$(date -Is)" >>"$output_dir/q-process-diagnostics.log"
	if test -n "$q_output"; then printf 'Q 診斷：%s\n' "$q_output" >>"$output_dir/q-process-diagnostics.log"; fi
}

wait_for_display
start_app
capture "$output_dir/01-title.png"

# Return：確認呈現方式那張卡（設定檔是新的，所以每次都會先問一次）；
# Space：關掉標題告示。
press Return
press space
# 空艙不會招來海盜（`PodDrawsEncounter`），沒有敵艦就沒有回火、船體不會歸零。
# 先下地面裝一批補給再回船：G 進地面、Escape 跳過落地、P 在起始倉庫裝補給、
# Right／Left 走開一步再走回船那一格登船。
press p
press g
press Escape
press p
press Right
press Left
# X：以正常玩家輸入進入戰術；O 關掉護盾，讓回火直接打在船體上。
press x
press o
capture "$output_dir/02-shields-off-retaliation.png"

# M 每次都是戰術移動與敵軍回應。敵艦不是每一拍都開火：裝填倒數由該型號的開火節奏
# 決定，命中與否再由距離與命中率決定，兩者都來自原版資料表。船體是原版的 16 點尺度，
# 距離也由敵艦自己的航向決定，因此擊沉需要的回合數由原版規則決定，不是由這個腳本決定：
# 作為接近失敗畫面。
# 需要幾回合由這一局的敵艦型號、方位與距離決定，不是固定值——早先寫的 90 回合
# 是當時那個生成序列的實測值。200 是留了餘裕的上界；船體歸零之後畫面就切到
# failure outcome，多按的 M 不再有作用。這條收據對生成序列仍然敏感。
for _ in $(seq 1 200); do
	press m
done
capture "$output_dir/03-near-failure-tactical.png"

# 第 91 次 M 使船體歸零，正常進入 failure outcome。
press m
capture "$output_dir/04-failure-outcome.png"

# D-OUT-01：failure outcome 是終局；直接按 S 保存這個終局，無返回續玩的中間畫面。
press s
capture "$output_dir/05-terminal-saved-outcome.png"
quit_app
wait_for_exit failure-outcome

test -s "$save_path"
grep -Eq '"outcomeKey"[[:space:]]*:[[:space:]]*"ui.outcome.failure"' "$save_path"
grep -Eq '"hullCondition"[[:space:]]*:[[:space:]]*0' "$save_path"
grep -Eq '"screen"[[:space:]]*:[[:space:]]*"outcome"' "$save_path"
grep -Eq '"saved"[[:space:]]*:[[:space:]]*true' "$save_path"
for image in 01-title.png 02-shields-off-retaliation.png 03-near-failure-tactical.png \
	04-failure-outcome.png 05-terminal-saved-outcome.png; do
	test -s "$output_dir/$image"
done
sha256sum "$output_dir"/*.png "$save_path"
