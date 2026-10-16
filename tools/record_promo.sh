#!/usr/bin/env bash
set -euo pipefail

# 錄推廣影片的 remake 段落：讓遊戲跑一段正常玩家路徑，以 ffmpeg x11grab 錄成 MP4。
# 規格：docs/spec/promo-video.md
#
# 此腳本必須在 sundog-remake-build Docker image 內、從 repo 根目錄執行：
#
#   docker run --rm --network none -v "$PWD":/work -w /work -v <輸出>:/out \
#     -e GOFLAGS=-buildvcs=false --entrypoint bash sundog-remake-build:20260812 \
#     tools/record_promo.sh /out
#
# [HARD] 腳本要以**檔案**的形式進容器，不能用 docker run -c '整段腳本'——
#   中文會在命令列參數那一層被打壞，xdotool type 會噴
#   「Invalid multi-byte sequence encountered」。replay_*.sh 一直是檔案形式，
#   所以它們的中文角色名輸入才會動。
#
# 只用正常玩家輸入（跟 tools/replay_*.sh 同一套按鍵），不用 debug 旗標、狀態注入、
# 座標／物品注入或預製存檔。影片是呈現，不是驗收——驗收在 replay 腳本與 Go／Python 測試。
#
# 成本（實測）：九段合計約 6 分鐘，加建置約 7 分鐘，單核加一個 ffmpeg。
# 給了 PROMO_ATARI_DISK 才會錄第十段（原版地表圖塊）；那一段要玩家自己的原版 .st。

out_dir=${1:?用法（容器內）：record_promo.sh /輸出目錄}
: "${PROMO_ATARI_DISK:?推廣片的 Atari ST 原版開場需要 PROMO_ATARI_DISK 指向玩家自備的 .st}"
test -f "$PROMO_ATARI_DISK"
mkdir -p "$out_dir/clips"

# 只錄指定段落：PROMO_SEGMENTS='seg7 seg9'。預設全錄。
# 改壞一段不必重錄六分鐘。
wanted="${PROMO_SEGMENTS:-all}"
want() {
	case " $wanted " in
		*" all "*) return 0 ;;
		*" $1 "*) return 0 ;;
	esac
	return 1
}

# [HARD] 容器預設 locale 是 POSIX，xdotool type 會把中文判成
#   「Invalid multi-byte sequence encountered」而整段中止。要顯式給 UTF-8。
export LC_ALL="${LC_ALL:-C.UTF-8}"
export PATH="/usr/local/go/bin:/go/bin:$PATH"
export GOCACHE="${SUNDOG_GOCACHE:-/tmp/promo-gocache}"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/promo-runtime}"
mkdir -p "$GOCACHE" "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"

promo_binary=${PROMO_BINARY:-/tmp/sundog-promo}
if test -n "${PROMO_BINARY:-}"; then
	test -x "$promo_binary"
else
	go build -buildvcs=false -o "$promo_binary" ./cmd/sundog-ebiten
fi

# 錄的是玩家看到的預設視窗。
scale="${PROMO_SCALE:-4}"
window_width=$((320 * scale))
window_height=$((200 * scale))

display_number="${PROMO_DISPLAY_NUMBER:-96}"
display=":${display_number}"
Xvfb "$display" -screen 0 1280x800x24 -nolisten tcp >/tmp/promo-xvfb.log 2>&1 &
xvfb_pid=$!
export DISPLAY="$display"

app_pid=""
ff_pid=""
window=""

cleanup() {
	if test -n "$ff_pid"; then kill "$ff_pid" 2>/dev/null || true; fi
	if test -n "$app_pid"; then kill -- "-$app_pid" 2>/dev/null || kill "$app_pid" 2>/dev/null || true; fi
	kill "$xvfb_pid" 2>/dev/null || true
	if test -n "$ff_pid"; then wait "$ff_pid" 2>/dev/null || true; fi
	if test -n "$app_pid"; then wait "$app_pid" 2>/dev/null || true; fi
	wait "$xvfb_pid" 2>/dev/null || true
}
trap cleanup EXIT

for _ in $(seq 1 50); do xwininfo -root >/dev/null 2>&1 && break; sleep 0.1; done

find_window() {
	local candidate
	for _ in $(seq 1 120); do
		candidate="$(xdotool search --onlyvisible --class ebitengine-application 2>/dev/null | head -n 1 || true)"
		if test -n "$candidate" && xwininfo -id "$candidate" 2>/dev/null | grep -q "Width: $window_width"; then
			printf '%s\n' "$candidate"
			return 0
		fi
		sleep 0.2
	done
	return 1
}

# 每一段都從 title 重新開機，跟 replay 腳本同樣的起點；存檔與保存庫每段丟棄，
# 段與段之間不會互相污染。
start_segment() {
	local name="$1"
	shift
	rm -f /tmp/promo-save.json /tmp/promo-library.json /tmp/promo-settings.json
	setsid "$promo_binary" -scale "$scale" -start title \
		-atari-disk "$PROMO_ATARI_DISK" \
		-settings /tmp/promo-settings.json -save /tmp/promo-save.json -library /tmp/promo-library.json \
		"$@" >"/tmp/promo-$name.log" 2>&1 &
	app_pid=$!
	window="$(find_window)"
	xdotool windowfocus --sync "$window"
	sleep 6.0
	# 每一段都從乾淨的設定檔起步，所以每一段都會先被問「要用哪一套呈現」。
	# 預設先答掉，讓各段的按鍵序列跟以前一樣；要讓卡片入鏡的那一段自己設
	# LOOK_ON_CAMERA=1，錄影開始之後再答。
	if test "${LOOK_ON_CAMERA:-0}" != 1; then
		press Return 1.0
		dismiss_title_notice
	fi
	local geometry
	geometry="$(xdotool getwindowgeometry --shell "$window")"
	eval "$geometry"
	# 只錄視窗那一塊，不錄整個 root；錄到的就是玩家看到的視窗。
	# 擷取端刻意近無損：畫面是像素美術，第一次編碼就把色度砍半的話，後面再怎麼調都
	# 救不回來。yuv444p 保留完整色度，crf 12 幾乎看不出損失，ultrafast 才跟得上 30 fps。
	# 這一份是中間檔，只活到合成為止。
	ffmpeg -v error -y -f x11grab -framerate 30 -video_size "${window_width}x${window_height}" \
		-i "${display}+${X},${Y}" -threads 2 -c:v libx264 -pix_fmt yuv444p -crf 12 -preset ultrafast \
		"$out_dir/clips/$name.mp4" >"/tmp/promo-ff-$name.log" 2>&1 &
	ff_pid=$!
	sleep 1.5
}

end_segment() {
	sleep 1.2
	# ffmpeg 要收到 SIGINT 才會把 moov atom 寫完；直接 kill 會留下讀不開的檔。
	kill -INT "$ff_pid" 2>/dev/null || true
	wait "$ff_pid" 2>/dev/null || true
	ff_pid=""
	kill -- "-$app_pid" 2>/dev/null || kill "$app_pid" 2>/dev/null || true
	wait "$app_pid" 2>/dev/null || true
	app_pid=""
	sleep 0.5
}

# 標題的告示（sign_on 的 set_timeout 迴圈）會吃掉下一個按鍵，而且那個鍵不會同時當成
# 選單動作。實測答完呈現卡之後告示還在，於是接下來的第一個 F8 會憑空消失。用一個
# 在遊戲裡沒有任何綁定的鍵把它關掉：`AppendJustPressedKeys` 收得到 F5，polledKeys 收不到，
# 所以不管告示在不在，這一下都不會誤觸任何動作。
dismiss_title_notice() {
	press F5 0.6
}

# 節奏：給人看的，比 replay 的 0.7 秒慢。
pace="${PROMO_PACE:-1.1}"
press() {
	window="$(find_window)"
	xdotool keydown --clearmodifiers --window "$window" "$1"
	sleep 0.18
	xdotool keyup --clearmodifiers --window "$window" "$1"
	sleep "${2:-$pace}"
}
type_char() {
	window="$(find_window)"
	xdotool type --clearmodifiers --window "$window" --delay 150 "$1"
	sleep 0.8
}

# 降落與起飛是疊在地面上的過場（XLANDFX），它自己一格一格推進，**期間其他按鍵會被吃掉**
# ——只有 Escape 與 Space 會把它跳過。所以按下 g 之後要等它走完再送下一個鍵，
# 否則接下來那一兩個動作會憑空消失。這是 2026-08-22 加上輪廓過場之後才有的事。
land_and_settle() {
	press g 1.6
	# 過場自己一格一格推進，長度隨負載而變：錄影時 Xvfb 是軟體渲染、x11grab 又同時
	# 在吃 CPU，實測會拖到二十秒以上——那在影片裡就是一整段沒有介面的綠色畫面。
	# 演兩秒給人看，然後主動跳過。Space 在過場期間就是「跳過」，而地面畫面本身
	# 不理會 Space，所以就算過場已經自己演完，這一下也不會誤觸任何動作。
	sleep 2.0
	press space 1.0
}

# 船艙的小人只在按住左鍵時走（XMOVEONS:0x01 xmove_on_ship 的 button(0,0) 分支），
# 放開的那一刻才判站位（check_enc）。所以示範要用「移到定點 → 按住 → 等它走 → 放開」，
# 不能用單次點擊。座標是 320×200 版面座標乘上 scale。
drag_walker() {
	local lx=$1 ly=$2 hold=${3:-3.0}
	window="$(find_window)"
	xdotool mousemove --sync --window "$window" $((lx * scale)) $((ly * scale))
	sleep 0.4
	xdotool mousedown 1
	sleep "$hold"
	xdotool mouseup 1
	sleep 1.2
}

# 中段幾段先切到重製版現代風，遊玩畫面才看得清楚。這是正常玩家操作（F8 兩下走完
# Atari ST → Apple II → 重製版現代風），不是預設值改動；seg6 才是完整的呈現示範。
switch_to_modern() {
	press F8 0.35
	press F8 0.6
}

if want seg1; then
	echo '== seg1 開場與角色'
	# 這一段讓開場的呈現卡入鏡：新玩家第一眼看到的就是它。
	LOOK_ON_CAMERA=1 start_segment seg1
	sleep 2.0
	# ENTER 取預設那一套（Atari ST 原版風）。
	press Return 1.6
	dismiss_title_notice
	press c
	press n
	# xdotool的Unicode合成鍵不等於玩家IME；錄影用原角色Zed，避免空白姓名。
	type_char 'Zed'
	sleep 1.0
	press Return
	for _ in $(seq 1 12); do press Right 0.12; done
	press Down 0.5
	for _ in $(seq 1 8); do press Right 0.12; done
	press Return 1.5
	press Return 2.0
	land_and_settle
	sleep 2.5
	end_segment
fi


if want seg2; then
	echo '== seg2 城市地面與交易'
	start_segment seg2
	switch_to_modern
	press p
	land_and_settle
	# 設施在**降落的那一格**上。先走再開設施的話會站到沒有設施的格子上，
	# 兩個動作都會被拒絕——舊版的腳本就是這樣，錄出來只有走路。
	press t 1.6
	press b 1.3
	press b 1.3
	press s 1.3
	press Escape 1.3
	press u 1.8
	sleep 1.4
	press Escape 1.3
	# 這一圈八格全部可通行，最後回到原地（實測 (1,15)→(3,17)→(1,15)）。
	# 八格也在街頭遭遇的第一次觸發（第 17 格）之前，所以不會被搭話打斷。
	for key in Right Right Down Down Left Left Up Up; do press "$key" 0.8; done
	sleep 1.0
	end_segment
fi


if want seg3; then
	echo '== seg3 船上與工程艙'
	start_segment seg3
	switch_to_modern
	press p 1.4
	press e 1.6
	sleep 1.6
	press d 1.3
	sleep 1.2
	press Escape
	press i 1.6
	sleep 1.6
	press Escape
	end_segment
fi


if want seg4; then
	echo '== seg4 系統內航行'
	start_segment seg4
	switch_to_modern
	press p
	press n 1.5
	press Down
	press Return 1.4
	for _ in $(seq 1 10); do press Return 0.45; done
	press e 2.0
	sleep 1.5
	end_segment
fi


if want seg5; then
	echo '== seg5 戰鬥'
	start_segment seg5
	switch_to_modern
	press p
	press x 1.6
	press f 1.2
	for _ in $(seq 1 10); do press r 0.4; press f 0.6; done
	sleep 1.5
	press Escape
	end_segment
fi


if want seg6; then
	echo '== seg6 切換呈現'
	start_segment seg6
	press p
	land_and_settle
	# F8走完目前兩種呈現（Atari ST／Apple II），回到起點；舊HD已退役。
	# 配色、圖塊與版面是一起換的：三者分開時，Atari ST 版面下另外兩軸畫不出差別，
	# 而其中一組會把重製版地圖浮貼在原版滿版城市上（2026-09-03 量測）。
	for _ in $(seq 1 2); do press F8 2.0; done
	sleep 1.2
	# 再走一輪，讓每一套都停久一點看清楚。
	for _ in $(seq 1 2); do press F8 2.6; done
	sleep 1.5
	end_segment
fi


if want seg7; then
	echo '== seg7 攜物格與指南針'
	start_segment seg7
	switch_to_modern
	press p
	land_and_settle
	# 起降格同時是倉庫，先在零件櫃買一件，攜物格才有東西可搬。
	# 往後選五格：櫃上前四件買不起，第五件是分流器（只加分流器數，不進攜物格）。
	press y 1.6
	for _ in $(seq 1 5); do press Right 0.5; done
	press b 1.6
	sleep 1.0
	press Escape 1.4
	# 走四個方向，指南針跟著轉；最後往北，指南針就指北。
	for key in Right Left Down Up; do press "$key" 0.9; done
	press Escape
	press h 1.8
	sleep 1.5
	# 角色狀態裡的 h 把東西在手上與身上之間搬。
	press h 1.6
	sleep 1.3
	press h 1.6
	sleep 1.3
	press Escape
	end_segment
fi


if want seg8; then
	echo '== seg8 導航三張圖'
	start_segment seg8
	switch_to_modern
	press p
	press n 1.8
	# 動哪一層，圖就換成那一層：星圖、星系圖、行星圖。
	press bracketright 1.6
	press bracketright 1.6
	press Down 1.6
	press Down 1.6
	press Right 1.6
	press Right 1.6
	sleep 1.4
	press Escape
	end_segment
fi


if want seg9; then
	echo '== seg9 貨艙與輔助燃料'
	start_segment seg9
	switch_to_modern
	press p
	land_and_settle
	# 起降格本身就是太空港，燃料槽在這裡買。
	press o 1.8
	sleep 1.2
	press Escape
	press c 1.8
	sleep 1.6
	press o 1.8
	sleep 1.4
	press Escape
	end_segment
fi


# 原版圖塊要玩家自己的磁碟；沒給就不錄這一段。這一段不切主題——
# 換成原版圖塊之後，畫面就該是原版那個樣子。
if test -n "${PROMO_ATARI_DISK:-}" && want seg10; then
	echo '== seg10 原版地表圖塊'
	start_segment seg10 -atari-disk "$PROMO_ATARI_DISK"
	press p
	land_and_settle
	for key in Right Right Up Left Down Down; do press "$key" 0.9; done
	sleep 1.8
	press Escape
	end_segment
elif want seg10; then
	echo '== seg10 跳過：沒有給 PROMO_ATARI_DISK'
fi

# 同一條路徑各走一次，只有呈現不同。預設是 Atari ST 原版風（呈現循環的第一個），
# Apple II 原版風是往前一格。
platform_walk() {
	press p
	land_and_settle
	for key in Right Right Down Left Up; do press "$key" 0.9; done
	sleep 1.0
	press Escape
	press h 1.8
	sleep 1.6
	press Escape
	press c 1.6
	sleep 1.4
	press Escape
}

if want seg11; then
	echo '== seg11 Apple II 樣式'
	start_segment seg11
	# 預設是 Atari ST 原版風，往前一格就是 Apple II 原版風。
	press F8 0.9
	platform_walk
	end_segment
fi

if want seg12; then
	echo '== seg12 Atari ST 樣式'
	start_segment seg12
	# 預設就是 Atari ST 原版風，所以這一段不按 F8。seg11 走的是 Apple II 原版風。
	platform_walk
	end_segment
fi

# Atari ST 介面：駕駛台、戰術顯示器、工程艙與補給站視窗，框與視窗位置都對過原版。
if want seg13; then
	echo '== seg13 Atari ST 介面'
	start_segment seg13
	# 預設呈現就是 Atari ST 原版風，所以這一段不按 F8。
	press p 1.6
	press n 2.2
	press bracketright 1.6
	press Escape
	press x 2.4
	press Escape
	press e 2.2
	press Escape
	land_and_settle
	press o 2.2
	press Escape
	end_segment
fi

if want seg14; then
	echo '== seg14 船艙走路人與走回船上'
	# 這一段全程用預設呈現（Atari ST 原版風）：船艙剖面與小人只在那個版面裡。
	start_segment seg14
	sleep 2.0
	# 標題先顯示告示，第一個鍵只用來關掉它。
	press space
	press p 2.0
	# 先示範地面：離艦、落地，地面上看得到停在落地格的船（XMOVEONG:0x07 put_ship）。
	land_and_settle
	sleep 1.5
	# 回船要走到船那一格：往東走開一步，再走回來就登船。
	press Right 1.6
	press Left 2.4
	# 拉到視窗 72（躍遷驅動的維修站，版面 129–145 × 29–37）。小人只在按住左鍵時走，
	# 放開的那一刻才判站位（XMOVEONS:0x0c check_enc）。
	drag_walker 137 33 3.5
	sleep 1.6
	press Escape 1.4
	# 拉向貨艙視窗 66（版面 21–30 × 58–73）。那一塊沒有可站的錨點，重製版改成走到
	# 與小人連通的最近可站點——放開時停在哪裡就進哪個站位，所以這一段擺在最後。
	drag_walker 25 65 4.0
	sleep 2.0
	end_segment
fi

if want seg15; then
	echo '== seg15 電車站'
	# 原版的電車站是城市地圖上自己的建築（門碼 18），不是 Uniteller 的一個選項。
	# 這一段走的是正常玩家路徑：降落 → 走到那棟建築 → 選城市 → 確認 → 搭車。
	start_segment seg15
	sleep 2.0
	press space
	press p 2.0
	land_and_settle
	sleep 1.2
	# Drahew 的降落格是 (1,0)、電車站的門在 (13,3)。這條路徑是 BFS 算出來的最短解，
	# 十五步全部可通行；十五步也在街頭遭遇的第一次觸發（第 17 格）之前，不會被搭話打斷。
	#
	# **方向鍵對到哪一軸要照 reducer 的 move() 讀**：North／South 動的是 Y，
	# West／East 動的是 X（`internal/game/reducer.go`）。所以「X 加 12、Y 加 3」
	# 是 Right ×12 加 Down ×3，不是反過來——第一次錄的時候寫反了，走到一格沒有
	# 電車站的地方，畫面上是「這一格上沒有電車站。」
	for _ in 1 2 3 4 5 6 7 8 9 10 11 12; do press Right 0.55; done
	for _ in 1 2 3; do press Down 0.55; done
	sleep 1.0
	# B 開電車站；1 選第一個城市，畫面換成確認頁（City／Planet／System／Cost）。
	press b 2.2
	press 1 2.4
	# ENTER 是 TRANSPORT：扣掉車資、人到另一座城市。
	press Return 3.0
	sleep 1.5
	end_segment
fi

ls -la "$out_dir/clips"
