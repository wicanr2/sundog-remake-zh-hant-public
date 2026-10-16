#!/usr/bin/env bash
set -euo pipefail

# 在城市地面放開左鍵的那一刻停住，把 check_exit 算出來的格座標讀出來。
#
# `tools/check_city_map_runtime.py` 已經證明 MAINLIB_G3a1 那塊 40×24 緩衝就是靜態的城市
# 地圖，但那還缺一環：**畫面上的位置對應到緩衝的哪一格**。捲動偏移與步距都是
# xmove_on_ground 的區域變數，從畫面看不出來，只能在執行時讀。
#
# 中斷點設在 XMOVEONG 段內 0x153e——那是 check_exit 把格座標算完、把腳下的碼正規化完
# （大於 15 減 16），而進建築的判定還沒開始的位置：
#
#   14e6:  sstl2                 L2 := 欄 = (捲動x + Gec[0].w1 / (0x20+Lf)) mod 40
#   14f9:  sstl1                 L1 := 列 = (捲動y + Gec[0].w2 / (L1c+Lf)) mod 24
#   1536:  slod1 0x36 … sto      腳下的碼 -= 16（僅當它大於 15）
#   153e:  ← 中斷點
#   158e:  …碼 > 9 或碼 = 5 就叫 check_building
#
# 段名必須是 XMOVEONG。GRNDLIB 是另一個真實存在的段，`b GRNDLIB <addr>` 會回報設定成功，
# 只是設在別的段的同一個位址上——設得成功不代表設對地方。
#
# 用法：tools/port_oracle_vigor_probe.sh <laanwj checkout> <disk.st> <工作目錄>

src=${1:?用法：port_oracle_grid_probe.sh <laanwj checkout> <disk.st> <工作目錄>}
disk=${2:?}
work=${3:?}
IMAGE=${SUNDOG_PORT_ORACLE_IMAGE:-sundog-port-oracle:20260811}
NAME=${SUNDOG_PORT_PROBE_NAME:-sundog-vigor-probe}
SEG=MAINLIB
BP=0x0c7d
BP2=0x0c8a
src=$(cd "$src" && pwd)
disk=$(cd "$(dirname "$disk")" && pwd)/$(basename "$disk")
mkdir -p "$work"/{build,shots}
work=$(cd "$work" && pwd)
cp "$disk" "$work/disk.st"
uidgid="$(id -u):$(id -g)"

echo "== 建 port（帶 psys_debugger）"
docker run --rm --network none -u "$uidgid" --log-opt max-size=10m --log-opt max-file=3 \
	--memory 4g --cpus 4 --pids-limit 256 \
	-v "$src":/src:ro -v "$work/build":/build -w /build \
	--entrypoint bash "$IMAGE" -c '
set -e
meson setup -Dpsys_debugger=true /build /src >/dev/null 2>&1 \
  || meson setup --reconfigure -Dpsys_debugger=true /build /src >/dev/null
ninja -C /build >/dev/null
test -x /build/src/sundog'

cat >"$work/hold.sh" <<'INNER'
#!/bin/bash
set -e
export DISPLAY=:97
Xvfb :97 -screen 0 1280x960x24 -nolisten tcp >/tmp/xvfb.log 2>&1 &
sleep 2
cd /w
rm -f /tmp/dbgin; mkfifo /tmp/dbgin
exec 3<>/tmp/dbgin
LIBGL_ALWAYS_SOFTWARE=1 ./build/src/sundog --renderer basic /w/disk.st <&3 >/w/shots/run.log 2>&1 &
sleep 25
win=$(xdotool search --onlyvisible --name -- "." | head -1)
xdotool windowfocus --sync "$win" 2>/dev/null || true
# SDL 的滑鼠焦點靠 EnterNotify；Xvfb 沒有視窗管理器，要把指標甩出視窗再移回來才會發生。
xdotool mousemove 640 10; sleep 0.5; xdotool mousemove 640 940; sleep 0.5
xdotool mousemove 640 500; sleep 0.5
click(){ xdotool mousemove "$1" "$2"; sleep 0.7; xdotool mousedown 1; sleep 0.25; xdotool mouseup 1; sleep "${3:-2}"; }
rclick(){ xdotool mousemove "$1" "$2"; sleep 0.7; xdotool mousedown 3; sleep 0.25; xdotool mouseup 3; sleep "${3:-3}"; }
walk_to(){ xdotool mousemove "$1" "$2"; sleep 1; xdotool mousedown 1; sleep "${3:-12}"; xdotool mouseup 1; sleep 10; }
click 890 807 3; click 1077 259 3
click 145 605; click 205 605; click 685 700 3
click 265 685 3
rclick 640 400 2
click 265 622 5; sleep 4; click 458 622 6; sleep 5
click 1077 259 10; sleep 20
import -window "$win" /w/shots/shipboard.png
echo "VIGOR-READY win=$win"
tail -f /dev/null
INNER
chmod +x "$work/hold.sh"

echo "== 走到地面（約 3 分半）"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" --network none -u "$uidgid" \
	--log-opt max-size=10m --log-opt max-file=3 --memory 2g --pids-limit 256 \
	-v "$work":/w -w /w --entrypoint bash "$IMAGE" /w/hold.sh >/dev/null
until docker logs "$NAME" 2>&1 | grep -q VIGOR-READY; do sleep 5; done

# 送一條指令並等 run.log 出現新內容。除錯器走 readline，一次灌多行會有指令被吃掉，
# 所以逐條送、逐條確認。
send() {
	local before after tries
	before=$(wc -c <"$work/shots/run.log")
	printf '%s\n' "$1" | docker exec -i "$NAME" tee /tmp/dbgin >/dev/null
	for tries in $(seq 1 40); do
		sleep 0.5
		after=$(wc -c <"$work/shots/run.log")
		[ "$after" -gt "$before" ] && return 0
	done
	echo "  （$1：逾時，log 沒有新輸出）" >&2
}

echo "== 設中斷點 $SEG:$BP（進入那條表達式前）與 $SEG:$BP2（sstl4 之後）"
docker exec -e DISPLAY=:97 "$NAME" bash -c "
win=\$(xdotool search --onlyvisible --name -- '.' | head -1)
xdotool key --window \"\$win\" d 2>/dev/null || xdotool key d"
sleep 3
send "b $SEG $BP"
send "c"

echo "== 等時鐘推進觸發中斷點"
sleep 12

echo "== 逐條讀 L3、Vigor 與 G3d 的四個位元組"
send "ldl 3"
send "ldl 4"
send "lao MAINLIB 0x3d"
send "x 0x1fe8"
send "c"
sleep 6
send "ldl 3"
send "x 0x1fe8"
send "c"

echo "== run.log 尾端"
tail -40 "$work/shots/run.log"
