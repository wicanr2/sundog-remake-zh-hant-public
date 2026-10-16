#!/usr/bin/env bash
set -euo pipefail

# 由一份「正常玩家路徑產生、停在 shipboard」的 schema 25 存檔，透過正式 Ebiten UI
# 產生兩份 Atari ST UX 收據：地面正常走一步，以及從工程格拿起零件後的角色卡。
# 不改 JSON、不注入座標／物品；原版 raw .st 只以唯讀方式供 runtime 解出呈現素材。

binary=${1:?usage: replay_atari_st_ux_fixtures.sh <binary> <disk.st> <normal-save.json> <out-dir>}
disk=${2:?}
normal_save=${3:?}
out=${4:?}
scale=3
display=:93
settings="$out/settings.json"
ground_save="$out/ground-step-save.json"
status_save="$out/status-items-save.json"
ground_fixture="$out/ground-step-fixture.json"
status_fixture="$out/status-items-fixture.json"
library="$out/library.json"

mkdir -p "$out"
test -z "$(find "$out" -mindepth 1 -maxdepth 1 -print -quit)"
printf '%s\n' '{"schema":1,"theme":"atari-st-original","tileset":"atari-st-tiles","layout":"atari-st"}' >"$settings"
cp "$normal_save" "$ground_save"
cp "$normal_save" "$status_save"

Xvfb "$display" -screen 0 1280x800x24 -nolisten tcp >"$out/xvfb.log" 2>&1 &
xvfb_pid=$!
app_pid=""
window=""
export DISPLAY="$display"

cleanup() {
	if test -n "$app_pid" && kill -0 "$app_pid" 2>/dev/null; then
		kill "$app_pid" 2>/dev/null || true
		wait "$app_pid" 2>/dev/null || true
	fi
	kill "$xvfb_pid" 2>/dev/null || true
	wait "$xvfb_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 50); do
	xwininfo -root >/dev/null 2>&1 && break
	sleep 0.1
done
xwininfo -root >/dev/null

find_window() {
	local candidate
	for _ in $(seq 1 120); do
		candidate="$(xdotool search --onlyvisible --class ebitengine-application 2>/dev/null | head -n 1 || true)"
		if test -n "$candidate" && xwininfo -id "$candidate" 2>/dev/null | grep -q 'Width: 960'; then
			printf '%s\n' "$candidate"
			return 0
		fi
		sleep 0.2
	done
	return 1
}

start_loaded() {
	local save=$1 log=$2
	"$binary" -scale "$scale" -load -atari-disk "$disk" -settings "$settings" \
		-save "$save" -library "$library" >"$log" 2>&1 &
	app_pid=$!
	window="$(find_window)"
	xdotool windowfocus --sync "$window"
	sleep 1
}

press() {
	xdotool keydown --clearmodifiers --window "$window" "$1"
	sleep 0.25
	xdotool keyup --clearmodifiers --window "$window" "$1"
	sleep 0.8
}

capture() {
	import -window "$window" "$1"
	test -s "$1"
}

stop_app() {
	# Q 可在 keydown 那一幀就關閉視窗；若仍走通用 press，後續 keyup 會對已
	# 消失的 X11 window 送事件，產生與產品無關的 BadWindow 假失敗。
	xdotool keydown --clearmodifiers --window "$window" q || true
	for _ in $(seq 1 50); do
		kill -0 "$app_pid" 2>/dev/null || break
		sleep 0.1
	done
	if kill -0 "$app_pid" 2>/dev/null; then
		return 125
	fi
	wait "$app_pid"
	app_pid=""
	window=""
}

# 正常玩家動作：shipboard → ground → 跳完落地 → 方向鍵走一步 → 走回船那一格登船 → save。
# 走了一格的證據是前後兩張畫面不同：船停在落地格上，走回去就登船，所以存檔裡的位置
# 會回到出發點，不能拿它當「有沒有走」的判準。
start_loaded "$ground_save" "$out/ground-step.log"
press g
press Escape
capture "$out/ground-before.png"
press Right
capture "$out/ground-after.png"
# 回船要走到船那一格（XMOVEONG:0x17 re_enter_ship）。剛才往東走了一步，
# 往西走回去踏上船的格子就登船；地面的 Escape 不再回船。
press Left
press s
cp "$ground_save" "$ground_fixture"
stop_app

if cmp -s "$out/ground-before.png" "$out/ground-after.png"; then
	echo "地面方向鍵沒有改變畫面" >&2
	exit 1
fi

python3 - "$normal_save" "$ground_fixture" <<'PY'
import json, sys
before = json.load(open(sys.argv[1], encoding="utf-8"))["state"]
after = json.load(open(sys.argv[2], encoding="utf-8"))["state"]
if after["screen"] != "shipboard":
    raise SystemExit(f"地面收據沒有回到 shipboard：{after['screen']}")
if after["position"] != after["shipPosition"] or after["cityId"] != after["shipCityId"]:
    raise SystemExit(f"登船的位置不是船那一格：{after['position']} vs {after['shipPosition']}")
if after["position"] != before["position"]:
    raise SystemExit(f"走一步再走回來之後位置不同：{before['position']} → {after['position']}")
print(json.dumps({"landing": before["position"], "boardedAt": after["shipPosition"]}, ensure_ascii=False))
PY

# 正常玩家動作：shipboard → engineering → 點第一格拿起既有零件 → shipboard → save → status。
start_loaded "$status_save" "$out/status-items.log"
press e
# show_system 第一格中心：(window 72,64) + inside 1 + (51,24)，3× 視窗座標。
xdotool mousemove --sync --window "$window" $((124 * scale)) $((89 * scale))
xdotool click --window "$window" 1
sleep 1
press Escape
press s
cp "$status_save" "$status_fixture"
press h
# Atari ST zoom 會把剛進畫面的訊息蓋在 readout 上約兩秒；等正式角色卡
# 內容恢復後再抓圖，不能用 Escape（那會正常返回 title）。
sleep 2.2
capture "$out/status-items.png"
stop_app

python3 - "$status_fixture" <<'PY'
import json, sys
state = json.load(open(sys.argv[1], encoding="utf-8"))["state"]
if state["handItem"] == 0:
    raise SystemExit("工程格點擊後手上仍無物品")
if state["screen"] != "shipboard":
    raise SystemExit(f"fixture 不是可由 H 開角色卡的 shipboard：{state['screen']}")
print(json.dumps({"handItem": state["handItem"]}, ensure_ascii=False))
PY

printf '%s\n' "$(basename "$status_fixture")" >"$out/status-items.fixture"
sha256sum "$ground_fixture" "$status_fixture" "$out/ground-before.png" "$out/ground-after.png" "$out/status-items.png"
