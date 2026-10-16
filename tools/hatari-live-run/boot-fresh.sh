#!/bin/sh
umask 000
export DISPLAY=:99
Xvfb :99 -screen 0 800x600x24 &
sleep 2
mkdir -p /out/shots && cd /out/shots
cp /in/sundog-a2.st /tmp/disk.st
hatari --machine st --tos /out/tos102.img --cmd-fifo /tmp/hfifo \
  --confirm-quit false --sound off --statusbar false --drive-led false /tmp/disk.st >/out/hatari.log 2>&1 &
n=0; until [ -p /tmp/hfifo ] || [ $n -gt 20 ]; do sleep 1; n=$((n+1)); done
sleep 12
xdotool mousemove 160 177; sleep 1; xdotool click --repeat 2 --delay 80 1; sleep 4
xdotool mousemove 224 249; sleep 1; xdotool click --repeat 2 --delay 80 1
sleep 85
echo BOOTED
tail -f /dev/null
