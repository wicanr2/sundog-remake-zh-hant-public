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
# 取戰術畫面的原版收據。
#
# 原版新遊戲的燃料是 0（D6 已證實），起飛會被 Out of fuel 擋下，所以正常路徑要先走進
# Depot 買燃料。這支跳過那一段：用除錯器把 MAINLIB 的全域 0xe5（tractor.gates 判斷
# Out of fuel 用的同一個）設起來，其餘照正常路徑走——駕駛台 → NAVIGATE → LIFTOFF，
# 然後在軌道上等隨機遭遇。
#
# **注入的只有燃料這一個值**，起飛、飛行與遭遇都是遊戲自己跑的。
#
# 站位 11／12／13 用三個不同的游標位置走，進去的畫面卻逐像素相同——因為走路人的落點不等於
# 游標位置（判定在放開左鍵那一刻，中間走到哪由通行判定決定）。在 XMOVEONS:0x0a get_location
# 下中斷點，停下時讀 GEMBIND_Gec[0] 的 w1／w2，就拿得到真正的落點。
#
# 用法：SUNDOG_STATION_XY="x y" tools/port_oracle_station_probe.sh <laanwj checkout> <disk.st> <工作目錄>

src=${1:?用法：port_oracle_grid_probe.sh <laanwj checkout> <disk.st> <工作目錄>}
disk=${2:?}
work=${3:?}
IMAGE=${SUNDOG_PORT_ORACLE_IMAGE:-sundog-port-oracle:20260811}
NAME=${SUNDOG_PORT_PROBE_NAME:-sundog-combat-probe}
SEG=XMOVEONS
BP=0x04de
FUEL=${SUNDOG_FUEL:-200}
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
echo "COMBAT-READY win=$win"
tail -f /dev/null
INNER
chmod +x "$work/hold.sh"

echo "== 走到地面（約 3 分半）"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" --network none -u "$uidgid" \
	--log-opt max-size=10m --log-opt max-file=3 --memory 2g --pids-limit 256 \
	-v "$work":/w -w /w --entrypoint bash "$IMAGE" /w/hold.sh >/dev/null
until docker logs "$NAME" 2>&1 | grep -q COMBAT-READY; do sleep 5; done

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

echo "== 進除錯器設燃料"
docker exec -e DISPLAY=:97 "$NAME" bash -c "
win=\$(xdotool search --onlyvisible --name -- '.' | head -1)
xdotool key --window \"\$win\" d 2>/dev/null || xdotool key d"
sleep 3
send "lao MAINLIB 0x5b"
send "dm /w/systems.bin"
send "lao MAINLIB 0xe5"
send "sro MAINLIB 0xe5 $FUEL"
# hatari-live-run.md 的「沒有遭遇不是機率，是貨艙」：update_movement 的四道閘門之一是
# 兩格貨艙之一要裝編號 2–30 的貨物（0 空、1 輔助燃料都不算），空船永遠不生成海盜。
send "sro MAINLIB 0xec 5"
# 船體在系統陣列的 byte 0xc0。新遊戲是 13–16，遭遇時撐不到開戰術顯示器。補到 40——
# 高於初始值但遠低於型別上限：寫 255 會讓 HULL 綠條溢出整個右側面板、六組燈號全黑，
# 那是把遊戲推進它從未進入過的狀態，不能拿來當原版行為的證據。
send "wb 0x20e4 40"
send "ldo MAINLIB 0xe5"
send "c"

if [ -z "${SUNDOG_SYSTEMS_ONLY:-}" ]; then
echo "== 走到駕駛台，起飛"
docker exec -e DISPLAY=:97 "$NAME" bash -c '
win=$(xdotool search --onlyvisible --name -- "." | head -1)
click(){ xdotool mousemove "$1" "$2"; sleep 0.7; xdotool mousedown 1; sleep 0.25; xdotool mouseup 1; sleep "${3:-2}"; }
walk(){ xdotool mousemove "$1" "$2"; sleep 1; xdotool mousedown 1; sleep "${3:-12}"; xdotool mouseup 1; sleep 8; }
walk 248 460                      # 站位 1：駕駛台
import -window "$win" /w/shots/c1-cockpit.png
click 178 193 3                   # NAVIGATE
import -window "$win" /w/shots/c2-nav-menu.png
click 178 302 8                   # LIFTOFF
import -window "$win" /w/shots/c3-liftoff.png
sleep 22
import -window "$win" /w/shots/c4-orbit.png
# 座標要用螢幕座標，不是截圖座標：截圖是 1280×800、Xvfb 是 1280×960，視窗從 y=80 起，
# 所以「截圖 y + 80 = 螢幕 y」。地面的 NAVIGATE 截圖在 114、螢幕是 193，正好差 80。
# 軌道選單五列的螢幕 y：NAVIGATE 194／MAPS 250／LAND 302／SUB-LIGHT 354／SET WARP 406。
click 178 354 5                   # SUB-LIGHT
import -window "$win" /w/shots/c5-sublight.png
# SUB-LIGHT 開的是星系內導航圖：中央是恆星、白點是行星、船的圖示在軌道上。
# 選目的地才會真的航行——航行中才會跑 encounterRating。
# 行星白點是 5×5 實心圓（放大 4 倍後是 20×20），要點兩下才選得到。
# 別跟躍遷點搞混：那些是 5×7（放大後 20×28），船是 6×8（24×32）。
# 座標由程式在 viewscreen 裡找 20×20 的白色連通區塊得到，不是目測。
click 821 341 2
click 821 341 6
import -window "$win" /w/shots/c6-target.png
sleep 25
import -window "$win" /w/shots/c7-cruise.png
sleep 45
import -window "$win" /w/shots/c8-cruise2.png
sleep 45
import -window "$win" /w/shots/c9-cruise3.png'
fi
sleep 2

echo "== 看有沒有遭遇（SHIPLIB_G4 是「有敵艦」旗標，G4d8 是型號）"
send "ldo SHIPLIB 0x4"
send "ldo MAINLIB 0xd8"

echo "== run.log 尾端"
tail -40 "$work/shots/run.log"
