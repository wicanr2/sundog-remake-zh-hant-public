#!/usr/bin/env bash
set -euo pipefail

# 用帶除錯器的 port 走到城市地面，然後把整份 p-system 記憶體 dump 出來。
# 目的是取得 MAINLIB_G3a1（每列 40 bytes、24 列的地面圖緩衝）——畫面上看不出哪一格的
# 地面碼落在 check_building 認的 10..20 裡，只能從記憶體讀。
#
# 用法：tools/port_oracle_dump.sh <laanwj checkout> <disk.st> <工作目錄>
#
# 除錯器走 readline，指令從一個 named pipe 餵進去（容器裡沒有 tty）。
# 進除錯器的方式是對 SDL 視窗按 D（src/sundog.c 的 SDLK_d）。

src=${1:?用法：port_oracle_dump.sh <laanwj checkout> <disk.st> <工作目錄>}
disk=${2:?}
work=${3:?}
IMAGE=${SUNDOG_PORT_ORACLE_IMAGE:-sundog-port-oracle:20260811}
NAME=${SUNDOG_PORT_DUMP_NAME:-sundog-port-dump}
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
# 除錯器用 readline 讀 stdin；容器裡沒有 tty，改用 named pipe，並且自己持有寫端
# 讓它不會在第一個指令之後看到 EOF。
rm -f /tmp/dbgin; mkfifo /tmp/dbgin
exec 3<>/tmp/dbgin
LIBGL_ALWAYS_SOFTWARE=1 ./build/src/sundog --renderer basic /w/disk.st <&3 >/w/shots/run.log 2>&1 &
game=$!
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
echo "DUMP-READY win=$win"
tail -f /dev/null
INNER
chmod +x "$work/hold.sh"

echo "== 走到地面（約 3 分半）"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" --network none -u "$uidgid" \
	--log-opt max-size=10m --log-opt max-file=3 --memory 2g --pids-limit 256 \
	-v "$work":/w -w /w --entrypoint bash "$IMAGE" /w/hold.sh >/dev/null
until docker logs "$NAME" 2>&1 | grep -q DUMP-READY; do sleep 5; done

echo "== 進除錯器並 dump"
docker exec -e DISPLAY=:97 "$NAME" bash -c '
win=$(xdotool search --onlyvisible --name -- "." | head -1)
xdotool key --window "$win" d 2>/dev/null || xdotool key d
sleep 2
{ echo "dm /w/psys-memory.bin"; sleep 2; echo "c"; } > /tmp/dbgin
sleep 3'
ls -la "$work/psys-memory.bin" 2>/dev/null || echo "沒有產生 dump——除錯器可能沒進去"
