#!/bin/sh
# 用 EmuTOS 開機並把原版 SunDog 跑到標題畫面（容器內執行）。
# 掛載約定與 boot-fresh.sh 相同：/in 是含 sundog-a2.st 的唯讀目錄，/out 是工作目錄，
# 且 /out 必須已有 emutos192us.img（由宿主端的 fetch-emutos.sh 取得）。
#
# 與 boot-fresh.sh 的差別只有兩處：ROM 換成 EmuTOS（不必持有版權 TOS），
# 以及桌面圖示座標換成 EmuDesk 的位置——TOS 1.02 桌面的圖示在畫面中段，
# EmuDesk 的在最上一列，沿用舊座標會點到選單列。
#
# 這條路到標題畫面為止：EmuTOS 下遊戲收不到滑鼠與鍵盤（兩條注入路徑都試過），
# 所以它適合做畫面的像素驗證，不能拿來操作遊戲。要互動走 boot-fresh.sh 配 TOS 1.02。
# 成因與證據見 docs/atari-st/tos-and-hatari.md。
umask 000
export DISPLAY=:99
Xvfb :99 -screen 0 800x600x24 &
sleep 2
mkdir -p /out/shots && cd /out/shots
cp /in/sundog-a2.st /tmp/disk.st
hatari --machine st --memsize 1 --tos /out/emutos192us.img --cmd-fifo /tmp/hfifo \
  --confirm-quit false --sound off --statusbar false --drive-led false /tmp/disk.st \
  >/out/hatari.log 2>&1 &
n=0; until [ -p /tmp/hfifo ] || [ $n -gt 20 ]; do sleep 1; n=$((n+1)); done
sleep 18
# DISK A（EmuDesk 最上一列左邊那個）
xdotool mousemove 161 144; sleep 1; xdotool click --repeat 2 --delay 80 1; sleep 4
# A:\ 視窗裡唯一的 SUNDOG.PRG
xdotool mousemove 203 261; sleep 1; xdotool click --repeat 2 --delay 80 1
# FTL 商標 splash 自己停 60–80 秒才轉標題，這裡留足餘量
sleep 140
echo BOOTED
tail -f /dev/null
