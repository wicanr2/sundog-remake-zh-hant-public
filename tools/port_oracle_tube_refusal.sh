#!/usr/bin/env bash
set -euo pipefail

# 取原版電車站「付不起車資」那條分支的畫面。
#
# `do_tube_station`（`XDOUNITE:0x0296`）在 `0x03b6` 拿 `MAINLIB:0x0e more` 比車資與現金：
# 付得起才開出 menu 6 框 1 的 `TRANSPORT`，付不起就在第 7 列印 `Insufficient funds`
# 然後回上一頁。probe 的起始狀態現金是 200–899（新遊戲的 `random() mod 700 + 200`），
# 而同行星最貴的一趟只要幾百，所以那條分支走不到——**得先把現金改小**。
#
# 改的是 `MAINLIB_G33`／`G34`（現金的高位與低位字，`money.base-1000`）。用除錯器的
# `sro` 而不是 `ww`：`sro` 自己查段的全域基底，不必在這裡重算一次位址。
#
# 前置：容器 `sundog-building-probe` 已經由 `port_oracle_building_probe.sh` 帶
# `SUNDOG_BUILDING_FACILITY=tube-station SUNDOG_BUILDING_SCROLL=1` 送進電車站的第一頁。
# 那支腳本跑完會把容器留著，這一支接手。
#
# 用法：port_oracle_tube_refusal.sh <工作目錄> [框的螢幕座標 x y]
#
# 邊界：只寫工作目錄；不清理任何 docker 資源（容器由呼叫端處置）。

work=${1:?用法：port_oracle_tube_refusal.sh <工作目錄> [x y]}
click_x=${2:-550}
click_y=${3:-730}
NAME=${SUNDOG_PORT_PROBE_NAME:-sundog-building-probe}

docker inspect "$NAME" >/dev/null 2>&1 || {
	echo "FAIL：容器 $NAME 不在。先跑 port_oracle_building_probe.sh 進電車站。" >&2
	exit 1
}

send(){ printf '%s\n' "$1" | docker exec -i "$NAME" tee /tmp/dbgin >/dev/null; sleep "${2:-1.5}"; }
last_value(){ sed 's/\x1b\[[0-9;]*m//g' "$work/shots/run.log" | grep -A1 -- "psys>>> $1\$" | tail -1; }

# 每次進出除錯器 SDL 都會掉滑鼠焦點；Xvfb 沒有視窗管理器，焦點只靠 EnterNotify 建立。
regain_focus='
win=$(xdotool search --onlyvisible --name -- "." | head -1)
xdotool windowfocus --sync "$win" 2>/dev/null || true
xdotool mousemove 640 10; sleep 1; xdotool mousemove 640 950; sleep 1'

echo "== 進除錯器，讀現在的現金"
docker exec -e DISPLAY=:97 "$NAME" bash -c "
win=\$(xdotool search --onlyvisible --name -- '.' | head -1)
xdotool key --window \"\$win\" d 2>/dev/null || xdotool key d
sleep 2"
send "ldo MAINLIB 0x33"; hi=$(last_value "ldo MAINLIB 0x33")
send "ldo MAINLIB 0x34"; lo=$(last_value "ldo MAINLIB 0x34")
printf '   現金 = %d × 1000 + %d = %d Cr\n' "$((hi))" "$((lo))" "$(( (hi) * 1000 + (lo) ))"

echo "== 把現金改成 1 Cr"
send "sro MAINLIB 0x33 0"
send "sro MAINLIB 0x34 1"
send "ldo MAINLIB 0x33"; hi2=$(last_value "ldo MAINLIB 0x33")
send "ldo MAINLIB 0x34"; lo2=$(last_value "ldo MAINLIB 0x34")
if [ "$((hi2))" != "0" ] || [ "$((lo2))" != "1" ]; then
	echo "FAIL：改完之後讀回 $((hi2))／$((lo2))，不是 0／1" >&2
	exit 1
fi
echo "   讀回 0 × 1000 + 1 = 1 Cr"

echo "== 放行，點第一個城市框"
send "c" 3
docker exec -e DISPLAY=:97 "$NAME" bash -c "
$regain_focus
xdotool mousemove $click_x $click_y; sleep 1
xdotool mousedown 1; sleep 0.3; xdotool mouseup 1
sleep 4
win=\$(xdotool search --onlyvisible --name -- '.' | head -1)
import -window \"\$win\" /w/shots/tube-refusal.png"

echo "== 結果"
echo "   $work/shots/tube-refusal.png"
