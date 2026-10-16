#!/usr/bin/env bash
set -euo pipefail

# 把走路人送進指定的城市建築，讓建築內的買賣能在同一次執行裡觀察。
#
# `port_oracle_grid_probe.sh` 只回答「腳下是哪一格」。要進建築就得**走**到那一格，而走路
# 靠滑鼠猜座標不可重複：走路人在畫面上的位置隨捲動而變，猜錯一次就會走出城市矩形，畫面
# 切成行星表面，之後每一步都建立在錯的狀態上。
#
# 這支改成用執行時的座標換算直接定位。`check_exit`（XMOVEONG:0x20）算格座標的式子是：
#
#   14d3:  slod1 0x3a; sind0        捲動x = *(lex1 local 0x3a)
#   14d6:  sldl4;      sind1        Gec[0].w1  ← 走路人的 x 像素
#   14d8:  ldcb 0x20; slod1 0xf; adi; dvi
#   14e6:  sstl2                    欄 = (捲動x + w1 / (32 + lex1[0xf])) mod 40
#   14e7:  slod1 0x39; sind0        捲動y = *(lex1 local 0x39)
#   14eb:  sind2                    Gec[0].w2  ← 走路人的 y 像素
#   14ec:  slod1 0x1c; slod1 0xf; adi; dvi
#   14f9:  sstl1                    列 = (捲動y + w2 / (lex1[0x1c] + lex1[0xf])) mod 24
#
# 這條式子可逆，所以「要站到某一格」有唯一解：
#
#   w1 = (目標欄 − 捲動x) × 步距x + 步距x/2
#   w2 = (目標列 − 捲動y) × 步距y + 步距y/2
#
# 三個量都在執行時讀，沒有一個是硬寫的：捲動偏移每局不同（實測 7 / 4），步距由兩個 local
# 相加而來（實測都是 32）。
#
# 進建築的閘門在 0x158e，與 `ground-rules.md` 記的一致：
#
#   158e:  slod1 0x36; sind0; sldc9; leqi; bnot    腳下的碼 > 9
#   1595:  slod1 0x36; sind0; sldc5; equi; lor     或 碼 = 5
#   159e:  scip1 0x1e                              → check_building
#
# 所以除了把走路人挪過去，還要讓這一次判定看到的三個量一致：L2（欄）、L1（列）、以及
# `*(lex1[0x36])`（腳下正規化後的碼）。三者都注入成目標格的真實值，不是繞過判定——
# 建築碼取自執行時緩衝該格的內容，不是編出來的。
#
# 用法：port_oracle_building_probe.sh <laanwj checkout> <disk.st> <工作目錄> [建築槽位]

src=${1:?用法：port_oracle_building_probe.sh <laanwj checkout> <disk.st> <工作目錄> [建築槽位]}
disk=${2:?}
work=${3:?}
slot=${4:-}
SUNDOG_BUILDING_SCROLL=${SUNDOG_BUILDING_SCROLL:-}
SUNDOG_BUILDING_FACILITY=${SUNDOG_BUILDING_FACILITY:-}
IMAGE=${SUNDOG_PORT_ORACLE_IMAGE:-sundog-port-oracle:20260811}
NAME=${SUNDOG_PORT_PROBE_NAME:-sundog-building-probe}
SEG=XMOVEONG
BP=0x153e
src=$(cd "$src" && pwd)
disk=$(cd "$(dirname "$disk")" && pwd)/$(basename "$disk")
mkdir -p "$work"/{build,shots}
work=$(cd "$work" && pwd)
cp "$disk" "$work/disk.st"
uidgid="$(id -u):$(id -g)"
here=$(cd "$(dirname "$0")" && pwd)

send(){ printf '%s\n' "$1" | docker exec -i "$NAME" tee /tmp/dbgin >/dev/null; sleep "${2:-1.5}"; }

# 讀 run.log 裡最後一個 `psys>>> <指令>` 之後那一行的值。
last_value(){ sed 's/\x1b\[[0-9;]*m//g' "$work/shots/run.log" | grep -A1 -- "psys>>> $1\$" | tail -1; }

# 每次進出除錯器，SDL 都會掉滑鼠焦點——Xvfb 沒有視窗管理器，焦點只靠 EnterNotify 建立，
# 而指標若已經在視窗裡就不會再發一次。所以每次動滑鼠前都要把指標甩出視窗再帶回來。
regain_focus='
win=$(xdotool search --onlyvisible --name -- "." | head -1)
xdotool windowfocus --sync "$win" 2>/dev/null || true
xdotool mousemove 640 10; sleep 1; xdotool mousemove 640 950; sleep 1
xdotool mousemove 710 480; sleep 1'

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
walk_to 296 528                       # 氣閘 → 城市地面
sleep 12
import -window "$win" /w/shots/ground.png
echo "PROBE-READY win=$win"
tail -f /dev/null
INNER
chmod +x "$work/hold.sh"

echo "== 走到城市地面（約 3 分半）"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" --network none -u "$uidgid" \
	--log-opt max-size=10m --log-opt max-file=3 --memory 2g --pids-limit 256 \
	-v "$work":/w -w /w --entrypoint bash "$IMAGE" /w/hold.sh >/dev/null
until docker logs "$NAME" 2>&1 | grep -q PROBE-READY; do sleep 5; done

echo "== 設中斷點 $SEG:$BP，走一步讓 check_exit 停下"
docker exec -e DISPLAY=:97 "$NAME" bash -c "
win=\$(xdotool search --onlyvisible --name -- '.' | head -1)
xdotool key --window \"\$win\" d 2>/dev/null || xdotool key d
sleep 2"
send "b $SEG $BP" 2
send "c" 2
docker exec -e DISPLAY=:97 "$NAME" bash -c "
$regain_focus
xdotool mousemove 710 560; sleep 1
xdotool mousedown 1; sleep 3; xdotool mouseup 1; sleep 5"

echo "== 讀執行時的座標換算"
send "ldl 1"; row=$(last_value "ldl 1")
send "ldl 2"; col=$(last_value "ldl 2")
send "ldl 4"; gec=$(last_value "ldl 4")
send "ups" ; send "ldl 0x3a"; px=$(last_value "ldl 0x3a")
send "ldl 0x39"; py=$(last_value "ldl 0x39")
send "ldl 0xf" ; f=$(last_value "ldl 0xf")
send "ldl 0x1c"; c1=$(last_value "ldl 0x1c")
send "ldl 0x36"; pcode=$(last_value "ldl 0x36")
send "downs" 1
send "dm /w/mem-ground.bin" 3

echo "   目前 (列 $((row)), 欄 $((col)))  &Gec[0]=$gec"
echo "   捲動x 指標 $px  捲動y 指標 $py  步距共用項 $((f))  步距y 另一項 $((c1))  腳下碼指標 $pcode"

echo "== 算目標建築格"
plan=$(python3 "$here/port_oracle_building_plan.py" \
	--memory "$work/mem-ground.bin" --gec "$gec" \
	--scroll-x-ptr "$px" --scroll-y-ptr "$py" \
	--stride-common "$f" --stride-y-extra "$c1" \
	--row "$row" --column "$col" ${slot:+--slot "$slot"} \
	${SUNDOG_BUILDING_FACILITY:+--facility "$SUNDOG_BUILDING_FACILITY"} \
	${SUNDOG_BUILDING_SCROLL:+--scroll})
echo "$plan"
eval "$(printf '%s\n' "$plan" | sed -n 's/^SET //p')"

echo "== 把走路人挪到門上，再點它自己一下"
# 只寫走路人的像素座標，L1／L2 與腳下的碼都讓 check_exit 自己重算——偽造那三個量會做出
# 「閘門通過但 check_building 判不到門」的假成功。點擊的位置就是走路人現在站的位置，
# `update_position` 把 sprite 朝游標移動，兩者重合所以位移是零。
# 視埠外的建築要先把地圖捲過去。捲動偏移與 sprite 座標是同一條換算的兩端，
# 一起寫才會自洽——只寫其中一個，check_exit 會算出第三個格子。
if [ -n "${TARGET_SCROLL_X:-}" ]; then
	send "ww $SCROLL_X_ADDR $TARGET_SCROLL_X"
	send "ww $SCROLL_Y_ADDR $TARGET_SCROLL_Y"
fi
send "ww $GEC_W1_ADDR $TARGET_W1"
send "ww $GEC_W2_ADDR $TARGET_W2"
send "c" 3
docker exec -e DISPLAY=:97 "$NAME" bash -c "
$regain_focus
xdotool mousemove $CLICK_X $CLICK_Y; sleep 1
xdotool mousedown 1; sleep 2; xdotool mouseup 1; sleep 5"

echo "== 驗證 check_exit 這次算到哪一格"
send "ldl 1"; row2=$(last_value "ldl 1")
send "ldl 2"; col2=$(last_value "ldl 2")
if [ "$((row2))" != "$TARGET_ROW" ] || [ "$((col2))" != "$TARGET_COL" ]; then
	echo "FAIL：check_exit 算到 (列 $((row2)), 欄 $((col2)))，不是目標 (列 $TARGET_ROW, 欄 $TARGET_COL)"
	echo "      走路人沒站上去，或點擊沒送達。容器 $NAME 保留著，可以接手查。"
	exit 1
fi
echo "   (列 $((row2)), 欄 $((col2))) 與目標相符"

echo "== 放行，讓 check_building 決定進不進得去"
send "db 0" ; send "c" 3
sleep 6
docker exec -e DISPLAY=:97 "$NAME" bash -c '
win=$(xdotool search --onlyvisible --name -- "." | head -1)
import -window "$win" /w/shots/building.png'

echo "== 結果"
python3 - "$work/shots/ground.png" "$work/shots/building.png" <<'''CMP'''
import sys
from PIL import Image, ImageChops
a = Image.open(sys.argv[1]).convert("RGB")
b = Image.open(sys.argv[2]).convert("RGB")
n = sum(1 for q in ImageChops.difference(a, b).getdata() if q != (0, 0, 0))
print(f"   與城市地面相異 {n} 像素" + ("　← 換了畫面" if n > 50000 else "　← 沒換畫面"))
CMP
echo "   $work/shots/building.png"
tail -12 "$work/shots/run.log" | sed 's/\x1b\[[0-9;]*m//g'
