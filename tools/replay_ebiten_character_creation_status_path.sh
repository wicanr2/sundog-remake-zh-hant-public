#!/usr/bin/env bash
set -euo pipefail

# `set -e` 之下失敗的 `test`／`grep` 不會印任何東西，看起來像「什麼都沒發生」。
# 把中止的行號吐出來，省得每次都要重跑加 -x 才知道停在哪。
trap 'status=$?; printf "腳本在第 %s 行以狀態 %s 中止\n" "$LINENO" "$status" >&2' ERR

# 這支重播的滑鼠座標與視窗判斷都是按 960×600 記下來的，所以明確給 -scale 3。
# 遊戲的預設倍率已改成 4（1280×800），但既有證據要維持原來的幾何才比得上。

# UI-002：從 title 以正常玩家輸入建立繁中角色，進入角色狀態，再返回船艦。
# 此腳本必須在 sundog-remake-build Docker image 內執行；不使用 direct-entry、
# state injection、預製 save、debug flag 或檔案改寫名稱。

output_dir="${1:-/tmp/sundog-ebiten-character-status}"
save_path="${2:-/tmp/sundog-ebiten-character-status-save.json}"
library_path="${3:-/tmp/sundog-ebiten-character-status-library.json}"
display_number="${SUNDOG_DISPLAY_NUMBER:-99}"
display_address=":${display_number}"
binary_path="${SUNDOG_EBITEN_BINARY:-/tmp/sundog-ebiten-character-status.bin}"
runtime_dir="${XDG_RUNTIME_DIR:-/tmp/sundog-ebiten-character-status-runtime}"

# 驗收必須從全新的 output／save／library 開始，避免混入舊證據或預製狀態。
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
export GOCACHE="${SUNDOG_GOCACHE:-/tmp/sundog-ebiten-character-status-gocache}"
export XDG_RUNTIME_DIR="$runtime_dir"
export LANG=C.utf8
export LC_ALL=C.utf8
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
	sleep 3
}

press() {
	window="$(find_window)" || {
		printf 'press %s：找不到視窗\n' "$1" >&2
		return 1
	}
	DISPLAY="$display_address" xdotool key --clearmodifiers --window "$window" "$1"
	sleep 0.7
}

type_text() {
	# 兩件事都會讓名稱悄悄留空，而且要到最後的存檔斷言才失敗：
	# 1. `press` 後面那 0.7 秒不夠——命名畫面還在切的時候送進去的字會被吞掉。
	#    這與中文無關，ASCII 一樣會掉。
	# 2. 兩個非 ASCII 字分兩次 `xdotool type` 送，第二個會掉。xdotool 為了打出
	#    keymap 裡沒有的字元會臨時佔用一個 keycode 再還原，連續兩次重映射之間
	#    GLFW 來不及更新它快取的鍵盤映射。一次把整個名字打完就只重映射一次。
	# 即使如此偶爾還是會整串掉，所以打完就量名稱欄那一塊的亮度：沒變亮就重打。
	# 門檻用「打字前後的差」而不是絕對值，才不會被主題或字型改動綁死。
	local before after attempt
	# 重打的次數給得寬：xdotool 的臨時 keymap 重映射與 GLFW 的快取之間是競態，
	# 三次不一定夠。
	for attempt in $(seq 1 8); do
		sleep 2
		window="$(find_window)"
		# 每一次都先把欄位清空再打。**只落地一個字**也是會發生的，那時名字會變成
		# 「陽」——畫面看起來有東西、Enter 也收，但存檔裡的名字是錯的，之後每一步
		# 都在對一個永遠不會成立的斷言空轉。
		DISPLAY="$display_address" xdotool key --clearmodifiers --window "$window" \
			BackSpace BackSpace BackSpace BackSpace
		sleep 0.5
		DISPLAY="$display_address" import -window "$window" "$output_dir/.name-probe.png"
		before="$(convert "$output_dir/.name-probe.png" -crop 120x30+195+280 -format '%[fx:mean]' info:)"
		DISPLAY="$display_address" xdotool type --clearmodifiers --window "$window" --delay 200 '陽犬'
		sleep 2
		window="$(find_window)"
		DISPLAY="$display_address" import -window "$window" "$output_dir/.name-probe.png"
		after="$(convert "$output_dir/.name-probe.png" -crop 120x30+195+280 -format '%[fx:mean]' info:)"
		rm -f "$output_dir/.name-probe.png"
		# 空欄位約 0.03、一個字約 0.07、兩個字約 0.11，所以門檻取 0.06 才分得出
		# 「只進了一個字」。
		if awk -v a="$after" -v b="$before" 'BEGIN { exit !(a - b > 0.06) }'; then
			return 0
		fi
		printf '名稱沒打進去（%s → %s），重打第 %s 次\n' "$before" "$after" "$attempt" >&2
	done
	printf '%s\n' '名稱欄一直是空的，放棄。' >&2
	return 1
}

capture() {
	local path="$1"
	local attempt
	# `import` 偶爾會抓到空檔（視窗正在重畫）。在 `set -e` 之下那會讓腳本靜默中止，
	# stdout、stderr 與診斷檔全空，看起來像「什麼都沒發生」。這裡重試並在真的
	# 失敗時出聲。
	for attempt in 1 2 3 4 5; do
		window="$(find_window)" || {
			printf 'capture %s：找不到視窗（第 %s 次）\n' "$path" "$attempt" >&2
			sleep 0.5
			continue
		}
		if DISPLAY="$display_address" import -window "$window" "$path" && test -s "$path"; then
			return 0
		fi
		printf 'capture %s：抓到空檔（第 %s 次）\n' "$path" "$attempt" >&2
		sleep 0.5
	done
	printf 'capture %s：五次都失敗\n' "$path" >&2
	return 1
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
start_app "$output_dir/app.log"
capture "$output_dir/01-title.png"

# Return：確認呈現方式那張卡（設定檔是新的，所以每次都會先問一次）；
# Space：關掉標題告示。
press Return
press space
# 正常玩家路徑：C → N → 文字輸入 → Return → H → Escape → Q。
press c
capture "$output_dir/02-library.png"
press n
type_text
capture "$output_dir/03-character-creation.png"
# 建立角色現在是三段：Enter 確認姓名 → 進屬性分配 → Enter 完成（440 點不分配，
# 與 D4 的原版 oracle 基線同一組起始狀態）→ 跳出「尚有點數未使用」的提醒 →
# 再一次 Enter 確認。舊腳本只有第一個 Enter，所以停在命名畫面、名稱還是空的。
press Return
sleep 1
capture "$output_dir/03z-attribute-allocation.png"

# 之後每一輪都是「Enter 往前推一步、S 試著存檔」。畫面切換有時比 press 的 0.7 秒
# 慢，固定按幾次會偶發落空；而 **Enter 在船艙畫面沒有綁定**，多按不會有副作用，
# 存檔落地就結束。用存檔本身當「已經到船艙了」的判準，比數按鍵可靠。
saved=0
for attempt in 1 2 3 4 5 6 7 8; do
	press Return
	press s
	for _ in $(seq 1 20); do
		if test -s "$save_path" &&
			grep -Eq '"character"[[:space:]]*:[[:space:]]*"陽犬"' "$save_path"; then
			saved=1
			break
		fi
		sleep 0.1
	done
	if test "$saved" -eq 1; then
		break
	fi
	printf '第 %s 輪還沒存檔落地，再推一次\n' "$attempt" >&2
done
capture "$output_dir/03y-character-created.png"
grep -Eq '"character"[[:space:]]*:[[:space:]]*"陽犬"' "$save_path"
capture "$output_dir/04-shipboard-new-character.png"
press h
capture "$output_dir/05-character-status.png"
press Escape
capture "$output_dir/06-returned-shipboard.png"

quit_app ui-002
q_fallback_used=0
if ! wait_for_exit ui-002; then
	q_fallback_used=1
fi

for image in 01-title.png 02-library.png 03-character-creation.png \
	04-shipboard-new-character.png 05-character-status.png 06-returned-shipboard.png; do
	test -s "$output_dir/$image"
done
sha256sum "$output_dir/01-title.png" "$output_dir/02-library.png" \
	"$output_dir/03-character-creation.png" "$output_dir/04-shipboard-new-character.png" \
	"$output_dir/05-character-status.png" "$output_dir/06-returned-shipboard.png"

if test "$q_fallback_used" -ne 0; then
	printf '%s\n' 'Q 未在 5 秒內正常收束；已執行有界 fallback kill，UI-002 判定失敗。' >&2
	exit 125
fi
