#!/usr/bin/env bash
set -euo pipefail

# 用公開的 laanwj/sundog port 當可互動的原版 oracle：建置、跑起來、走到指定畫面、截圖。
# 這條路不需要 Atari TOS ROM——port 自己解讀 p-system 磁碟，所以 D4 的實跑不再綁版權物。
#
# 用法：tools/port_oracle_capture.sh <laanwj checkout> <disk.st> <工作目錄> [階段]
#   階段 title（預設）／librarian／newgame／shipboard／cockpit／locker／pick／carry／repair／navigate／liftoff／ground／building／ground-hold
#
# ground-hold 走到城市地面之後不退出，容器留著讓宿主端用 docker exec 做閉迴路控制
# （見 tools/port_oracle_walk.py）。用它時記得 docker run 要帶 -d 與 --name。
#
# shipboard 會建一個名叫 Ab、屬性完全不分配的角色再進遊戲，所以每次跑得到同一個起始狀態。
#
# 邊界：upstream source 唯讀掛載，不複製進 repository；截圖含原版像素，留在工作目錄。
# 版本：research commit e56d8e2532134f0741a7f82b80f9357776779f27（見 docker/sundog-port-oracle/README.md）。

src=${1:?用法：port_oracle_capture.sh <laanwj checkout> <disk.st> <工作目錄> [階段]}
disk=${2:?}
work=${3:?}
stage=${4:-title}
IMAGE=${SUNDOG_PORT_ORACLE_IMAGE:-sundog-port-oracle:20260811}
src=$(cd "$src" && pwd)
disk=$(cd "$(dirname "$disk")" && pwd)/$(basename "$disk")
mkdir -p "$work"/{build,shots}
work=$(cd "$work" && pwd)
cp "$disk" "$work/disk.st"

cat >"$work/stage.sh" <<'INNER'
#!/bin/bash
set -e
export DISPLAY=:97
Xvfb :97 -screen 0 1280x960x24 -nolisten tcp >/tmp/xvfb.log 2>&1 &
xvfb=$!
trap 'kill $xvfb 2>/dev/null || true' EXIT
sleep 2
cd /w
LIBGL_ALWAYS_SOFTWARE=1 ./build/src/sundog --renderer basic /w/disk.st >/w/shots/run.log 2>&1 &
game=$!
sleep 25
win=$(xdotool search --onlyvisible --name -- "." | head -1)
xdotool windowfocus --sync "$win" 2>/dev/null || true

# SDL 的 mouse focus 靠 EnterNotify。視窗是建在指標下方的，指標從一開始就在裡面，
# 於是那個事件永遠不會發生，SDL_GetMouseState 也就不更新——點下去毫無反應，
# 畫面卻一切正常。先把指標甩出視窗再移回來，這一步不能省。
xdotool mousemove 640 10; sleep 0.5; xdotool mousemove 640 940; sleep 0.5
xdotool mousemove 640 500; sleep 0.5

# 視窗在螢幕 (0,80)，畫面 1280x800 是 320x200 的 4 倍：
#   螢幕座標 = (ST_x * 4, ST_y * 4 + 80)
click() { xdotool mousemove "$1" "$2"; sleep 0.8; xdotool mousedown 1; sleep 0.3; xdotool mouseup 1; sleep "${3:-3}"; }

shot() { import -window "$win" "/w/shots/$1"; }

rclick(){ xdotool mousemove "$1" "$2"; sleep 0.7; xdotool mousedown 3; sleep 0.25; xdotool mouseup 3; sleep "${3:-2}"; }

shot 00-title.png
if [ "$STAGE" = title ]; then
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
click 890 807 3                            # Select Game
shot 01-librarian.png
if [ "$STAGE" = librarian ]; then
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
click 1077 259 3                           # New Game
shot 02-name-entry.png
if [ "$STAGE" = newgame ]; then
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
# 名字用固定的 "Ab"：點字母格才會打字，+ 那一格完成
click 145 605                              # a
click 205 605                              # b
click 685 700 3                            # +
shot 03-name-confirm.png
click 265 685 3                            # Yes（名字正確）
shot 04-attributes.png
rclick 640 400 2                           # 右鍵叫出 Done／Adjust
shot 05-done-menu.png
click 265 622 5                            # Done（440 點完全不分配）
sleep 4
click 458 622 6                            # Keep this game? → Yes
sleep 5
shot 06-new-game-card.png
click 1077 259 10                          # Play SunDog
sleep 20
shot 07-shipboard.png
if [ "$STAGE" = shipboard ]; then
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
# 駕駛台是視窗 65，ST 矩形 (60,90)-(64,101)，中心 (62,95) → 螢幕 (248,460)。
# 小人只在左鍵按住時走，站位在放開那一刻才判（docs/atari-st/shipboard-walker.md）。
walk_to() {  # walk_to <螢幕x> <螢幕y> [按住秒數]
	xdotool mousemove "$1" "$2"; sleep 1
	xdotool mousedown 1; sleep "${3:-12}"; xdotool mouseup 1; sleep 10
}
if [ "$STAGE" = locker ]; then
	# 站位碼 5 是視窗 69，ST (103,101)-(111,112)，中心 (107,106) → 螢幕 (428,504)。
	# 實跑進去的是「角色狀態視窗＋Ship's Locker」，不是修理畫面——
	# shipboard-walker.md 寫的「站位碼 5–13 其餘都是 do_repairs」在這一格不成立。
	walk_to 428 504
	shot 08-locker.png
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
if [ "$STAGE" = pick ]; then
	walk_to 428 504
	shot 08-locker.png
	# 櫃子裡第一個 shunt 的圖示在 ST (117,133)，16×16 的中心是 (125,141) → 螢幕 (500,644)。
	# 點一下是「選取」：訊息列印出品名 shunt，物品仍留在櫃子裡，不是拿到身上。
	click 500 644 3
	shot 09-picked.png
	sleep 3
	shot 10-picked-settled.png
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
if [ "$STAGE" = carry ]; then
	walk_to 428 504
	shot 08-locker.png
	click 500 644 3                    # 選取櫃子裡第一個 shunt
	shot 09-picked.png
	# 角色左側的五個口袋 +；最上面那個在 ST (97,74) 附近 → 螢幕 (388,376)
	click 388 376 3
	shot 10-pocket-1.png
	sleep 3
	shot 11-carry.png
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
if [ "$STAGE" = station ]; then
	# 逐一驗證船艙站位。站位碼 = get_location 回傳值 = 視窗編號 − 0x40；
	# 座標取自 window-menu-tables.json 的視窗矩形中心，換算是
	# 螢幕 x = ST x × 4、螢幕 y = ST y × 4 + 80（用已驗過的站位 5 與 8 校準過）。
	#
	# 一個容器只走一個站位。連續走訪會拍到殘留畫面——那正是先前那張站位對應表不可靠的原因。
	case "${SUNDOG_STATION:?station 階段要設 SUNDOG_STATION}" in
	1)  sx=248; sy=460 ;;   # 視窗 65 ST (62,95)   駕駛台
	2)  sx=100; sy=340 ;;   # 視窗 66 ST (25,65)
	4)  sx=296; sy=528 ;;   # 視窗 68 ST (74,112)
	5)  sx=428; sy=504 ;;   # 視窗 69 ST (107,106) Ship's Locker（已驗）
	6)  sx=296; sy=368 ;;   # 視窗 70 ST (74,72)
	7)  sx=152; sy=184 ;;   # 視窗 71 ST (38,26)
	8)  sx=548; sy=212 ;;   # 視窗 72 ST (137,33)  Warp Drives（已驗）
	9)  sx=600; sy=240 ;;   # 視窗 73 ST (150,40)
	10) sx=548; sy=272 ;;   # 視窗 74 ST (137,48)
	11) sx=548; sy=676 ;;   # 視窗 75 ST (137,149)
	12) sx=600; sy=704 ;;   # 視窗 76 ST (150,156)
	13) sx=548; sy=736 ;;   # 視窗 77 ST (137,164)
	*)  echo "站位碼 $SUNDOG_STATION 不在 get_location 的範圍（1–13，且 3 被明文跳過）" >&2; exit 2 ;;
	esac
	# 站位矩形很小，而走路人會被艙內的隔板擋住——瞄準中心不一定停得進去。
	# SUNDOG_STATION_XY 可以覆寫成「x y」自己挑接近點。
	if [ -n "${SUNDOG_STATION_XY:-}" ]; then
		set -- $SUNDOG_STATION_XY; sx=$1; sy=$2
	fi
	if [ -n "${SUNDOG_STATION_PATH:-}" ]; then
		# 分段移動：走路人是朝游標的絕對位置走、碰到不可走的顏色就停，所以一次瞄準終點
		# 會被隔板擋住。先走到中繼點再轉向，可以繞過去。
		set -- $SUNDOG_STATION_PATH
		while [ $# -ge 2 ]; do
			walk_to "$1" "$2" 8
			shift 2
		done
	else
		walk_to "$sx" "$sy"
	fi
	shot "station-$SUNDOG_STATION.png"
	sleep 4
	shot "station-$SUNDOG_STATION-settled.png"
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE=$SUNDOG_STATION"
	exit 0
fi

if [ "$STAGE" = repair ]; then
	# 站位碼 8 是視窗 72，ST (129,29)-(145,37)，中心 (137,33) → 螢幕 (548,212)。
	# 進去的是 Warp Drives 維修畫面：4×4 網格，每列 control node／flux modulator／
	# photon bridge／s/t distorter 各一。
	walk_to 548 212
	shot 08-repair.png
	sleep 4
	shot 09-repair-settled.png
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
if [ "$STAGE" = ground ] || [ "$STAGE" = building ] || [ "$STAGE" = ground-hold ]; then
	# 氣閘是站位碼 4／視窗 68，ST (68,108)-(80,116)，中心 (74,112) → 螢幕 (296,528)。
	# 新遊戲時船停在 Drahew 城，所以出氣閘就落到城市地面。
	walk_to 296 528
	shot 08-airlock.png
	sleep 12
	shot 09-ground.png
	if [ "$STAGE" = ground-hold ]; then
		echo "GROUND-HOLD win=$win"
		tail -f /dev/null
	fi
	if [ "$STAGE" = building ]; then
		# 街頭遭遇是隨機的（每個迴圈迭代擲 random() mod 1000 == 21），走得越久越容易碰到，
		# 對話框一出來就擋掉後面的操作。先點兩次 No 的位置把它關掉——沒有對話框時
		# 那裡是地面，點一下只會讓走路人走一小步。
		click 900 567 2
		click 900 567 2
		shot 09b-after-dismiss.png
		# 放開左鍵時要同時滿足兩件事（docs/atari-st/ground-rules.md）：
		# 腳下那一格的地面碼 > 9，而且格內偏移落在該建築的門位置 ±4 內。
		# 顯示格 40 像素見方、內縮 4 之後是 32×32 圖塊；門多半在 (13,21) 一帶。
		# 建築在最後一列（顯示格列 4，ST y 160-199），不是列 3——列 3 是建築前的走道。
		# 顯示格 (5,4)：格左上 ST (200,160) + 內縮 4 + 門 (13,21) = ST (217,185)
		# → 螢幕 (217×4, 185×4+80) = (868,820)。
		walk_to 868 820 16
		shot 10-building.png
		sleep 6
		shot 11-building-settled.png
	fi
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
walk_to 248 460
shot 08-cockpit.png
if [ "$STAGE" = navigate ] || [ "$STAGE" = liftoff ]; then
	# 駕駛台左上的 NAVIGATE 按鈕在 ST (44,28) 附近 → 螢幕 (178,193)
	click 178 193 3
	# 展開後是 MAPS／LIFTOFF／SET WARP（船在地面時第二列是 LIFTOFF，在軌道上是 SUB-LIGHT）
	shot 09-navigate-menu.png
	if [ "$STAGE" = liftoff ]; then
		click 178 302 6            # LIFTOFF（選單第二列）
		shot 10-liftoff.png
		sleep 12
		shot 11-orbit.png
		sleep 10
		shot 12-orbit-settled.png
		kill $game 2>/dev/null || true
		echo "STAGE-DONE $STAGE"
		exit 0
	fi
	click 178 250 4                    # MAPS
	shot 10-maps.png
	sleep 4
	shot 11-maps-settled.png
	kill $game 2>/dev/null || true
	echo "STAGE-DONE $STAGE"
	exit 0
fi
kill $game 2>/dev/null || true
echo "STAGE-DONE $STAGE"
INNER
chmod +x "$work/stage.sh"

uidgid="$(id -u):$(id -g)"
common=(--rm --network none -u "$uidgid" --log-opt max-size=10m --log-opt max-file=3)

echo "== 建置 port（source 唯讀）"
docker run "${common[@]}" --memory 4g --cpus 4 --pids-limit 256 \
	-v "$src":/src:ro -v "$work/build":/build -w /build \
	--entrypoint bash "$IMAGE" -c '
set -e
meson setup /build /src >/dev/null 2>&1 || meson setup --reconfigure /build /src >/dev/null
ninja -C /build >/dev/null
test -x /build/src/sundog'

echo "== 跑到 $stage"
if [ "$stage" = ground-hold ]; then
	# 長駐：容器留著讓宿主端做閉迴路控制，所以不能帶 --rm，也不能前景等它
	name=${SUNDOG_PORT_ORACLE_NAME:-sundog-port-ground}
	docker rm -f "$name" >/dev/null 2>&1 || true
	docker run -d --name "$name" --network none -u "$uidgid" \
		--log-opt max-size=10m --log-opt max-file=3 \
		--memory 2g --pids-limit 256 \
		-v "$work":/w -w /w -e "STAGE=$stage" \
		--entrypoint bash "$IMAGE" /w/stage.sh >/dev/null
	echo "容器 $name 已在背景啟動；等它印出 GROUND-HOLD 再開始控制"
	until docker logs "$name" 2>&1 | grep -q GROUND-HOLD; do sleep 5; done
	docker logs "$name" 2>&1 | tail -1
else
	docker run "${common[@]}" --memory 2g --pids-limit 256 \
		-e SUNDOG_STATION="${SUNDOG_STATION:-}" \
		-e SUNDOG_STATION_XY="${SUNDOG_STATION_XY:-}" \
		-e SUNDOG_STATION_PATH="${SUNDOG_STATION_PATH:-}" \
		-v "$work":/w -w /w -e "STAGE=$stage" \
		--entrypoint bash "$IMAGE" /w/stage.sh
fi

ls -1 "$work/shots"/*.png
