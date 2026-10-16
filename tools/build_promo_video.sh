#!/usr/bin/env bash
set -euo pipefail

# 把 remake 段落、原版錄影與繁中說明卡合成一支推廣影片。
# 規格：docs/spec/promo-video.md
#
# 前置：
#   tools/record_promo.sh            → <out>/remake/clips/segN.mp4
#   tools/record_original_oracle.sh  → <out>/original/original-boot.avi
#
# 產物含原版畫面：只寫到 dist-all，不進版控、不進 release。
#
# 用法：tools/build_promo_video.sh /絕對/promo目錄
#
# 成本（實測）：抽幀與轉檔約 1 分鐘，單核。

promo_dir=${1:?用法：build_promo_video.sh /絕對/promo目錄}
if [[ "$promo_dir" != /* ]]; then
	echo "promo 目錄必須是絕對路徑。" >&2
	exit 2
fi

MEDIA_IMAGE=${SUNDOG_MEDIA_IMAGE:-u5cht/dev:latest}
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# 音效段用的是實際轉出來的檔（sundog-sound-export 的輸出）。沒給就退回無聲的卡。
# 那些檔屬於玩家自己的原版磁碟，不進版控也不進 release。
AUDIO_DIR=${PROMO_AUDIO_DIR:-}
audio_mount=()
if test -n "$AUDIO_DIR"; then
	test -d "$AUDIO_DIR"
	audio_mount=(-v "$AUDIO_DIR":/audio:ro)
fi

# 原版錄影的時間軸（11000 幀、60.04 fps，逐幀掃出來的）：
#   n=0        TOS desktop
#   n=800–4000 FTL logo
#   n=4800     標題畫面與 1985 年版權框
#   n=5600 起  主選單「Hello. You must select a game to play.」
ORIGINAL_TITLE_FRAME=${PROMO_ORIGINAL_TITLE_FRAME:-4800}
ORIGINAL_MENU_FRAME=${PROMO_ORIGINAL_MENU_FRAME:-6400}
# 放進影片的區間（秒）。從標題畫出來之前起算，到主選單出現為止，再以 2 倍速播——
# 原版這一段大半是靜止畫面，原速看太久。
ORIGINAL_CLIP_START=${PROMO_ORIGINAL_CLIP_START:-70}
ORIGINAL_CLIP_SECONDS=${PROMO_ORIGINAL_CLIP_SECONDS:-26}
ORIGINAL_CLIP_SPEED=${PROMO_ORIGINAL_CLIP_SPEED:-0.5}

# 成片解析度。挑 2560×1600 是因為兩邊都剛好是整數倍：重製版錄在預設的 1280×800，
# 放大兩倍；原版 640×400 放大四倍。非整數倍會把像素邊界磨成灰階，那是這種畫面上
# 最看得出來的一項；1280×800 雖然也是整數倍，但當推廣片偏小。
OUT_WIDTH=${PROMO_OUT_WIDTH:-2560}
OUT_HEIGHT=${PROMO_OUT_HEIGHT:-1600}
OUT_CRF=${PROMO_OUT_CRF:-16}
OUT_PRESET=${PROMO_OUT_PRESET:-medium}

test -f "$promo_dir/original/original-boot.avi"
test -d "$promo_dir/remake/clips"
mkdir -p "$promo_dir/stills" "$promo_dir/cards" "$promo_dir/parts"

docker run --rm --network none \
	--log-opt max-size=10m --log-opt max-file=3 \
	-v "$repo":/work:ro -w /work -v "$promo_dir":/promo "${audio_mount[@]}" \
	--entrypoint bash "$MEDIA_IMAGE" -c "
set -euo pipefail

# 1. 原版靜態畫面：標題與主選單。
ffmpeg -v error -y -i /promo/original/original-boot.avi \
	-vf \"select=eq(n\\,$ORIGINAL_TITLE_FRAME)\" -vsync 0 -frames:v 1 /promo/stills/original-title.png
ffmpeg -v error -y -i /promo/original/original-boot.avi \
	-vf \"select=eq(n\\,$ORIGINAL_MENU_FRAME)\" -vsync 0 -frames:v 1 /promo/stills/original-menu.png

# 2. 原版影片段落：正規化成跟 remake 一樣的尺寸與 30fps，兩者才接得起來。
#    原版是 640×400（4:3 的 ST 低解析），等比放大後左右補黑邊，不拉伸。
ffmpeg -v error -y -ss $ORIGINAL_CLIP_START -t $ORIGINAL_CLIP_SECONDS \
	-i /promo/original/original-boot.avi \
	-f lavfi -i anullsrc=channel_layout=stereo:sample_rate=44100 \
	-vf \"setpts=$ORIGINAL_CLIP_SPEED*PTS,scale=$OUT_WIDTH:$OUT_HEIGHT:flags=neighbor,fps=30\" \
	-shortest -c:v libx264 -pix_fmt yuv420p -crf $OUT_CRF -preset $OUT_PRESET \
	-c:a aac -b:a 128k /promo/parts/original-clip.mp4

# 3. remake 靜態畫面：標題與城市地面，供並列卡使用。
ffmpeg -v error -y -i /promo/remake/clips/seg1.mp4 -vf 'select=eq(n\,20)' -vsync 0 -frames:v 1 \
	/promo/stills/remake-title.png
ffmpeg -v error -y -i /promo/remake/clips/seg2.mp4 -vf 'select=eq(n\,150)' -vsync 0 -frames:v 1 \
	/promo/stills/remake-ground.png

# 4. 說明卡。
python3 /work/tools/build_promo_cards.py \
	--font /work/assets/fonts/NotoSansCJKtc-Regular.otf \
	--shots /promo/stills --output /promo/cards >/dev/null

# 5. 每張卡轉成一小段影片，長度依字數給。
# 每一段都帶一條音軌——只有一段有聲音，但 concat -c copy 要求所有段的串流一致。
make_card() {
	ffmpeg -v error -y -loop 1 -t \"\$2\" -i \"/promo/cards/\$1.png\" \
		-f lavfi -i anullsrc=channel_layout=stereo:sample_rate=44100 -shortest \
		-vf \"scale=$OUT_WIDTH:$OUT_HEIGHT:flags=lanczos,fps=30,format=yuv420p\" \
		-c:v libx264 -crf $OUT_CRF -preset $OUT_PRESET -c:a aac -b:a 128k \
		\"/promo/parts/\$1.mp4\"
}
make_card 01-title 3.5
make_card 02-what-is-this 7
make_card 03-world-data 7.5
make_card 04-tiles-palette 7.5
make_card 05-systems 7.5
make_card 06-presentation 7
make_card 07-restored 8.5
make_card 08-original-tiles 7.5
make_card 13-platform-looks 8
make_card 14-atari-st-interface 8
make_card 10-side-by-side 7
make_card 11-side-by-side-2 7
make_card 12-closing 4

# 音效段：把選定的幾段接成一條音軌，配上 09-sound 這張卡。
if test -d /audio; then
	# 段與段之間插半秒空白，聽的人才分得出這是八段而不是一段。
	ffmpeg -v error -y -f lavfi -i anullsrc=channel_layout=stereo:sample_rate=44100 \
		-t 0.5 -c:a pcm_s16le /tmp/gap.wav
	: >/tmp/sounds.txt
	for name in menu_click laser explosion teleport_up fuel_here hail warp2 attack_siren; do
		file=\$(ls /audio/*-\${name}.ogg /audio/*-\${name}.wav 2>/dev/null | head -n 1 || true)
		if test -n \"\$file\"; then
			printf \"file '%s'\\n\" \"\$file\" >>/tmp/sounds.txt
			printf \"file '/tmp/gap.wav'\\n\" >>/tmp/sounds.txt
		fi
	done
	if test -s /tmp/sounds.txt; then
		ffmpeg -v error -y -f concat -safe 0 -i /tmp/sounds.txt \
			-af 'aresample=44100' -ac 2 -c:a aac -b:a 128k /promo/parts/sound-track.m4a
		ffmpeg -v error -y -loop 1 -i /promo/cards/09-sound.png -i /promo/parts/sound-track.m4a \
			-shortest -vf \"scale=$OUT_WIDTH:$OUT_HEIGHT:flags=lanczos,fps=30,format=yuv420p\" \
			-c:v libx264 -crf $OUT_CRF -preset $OUT_PRESET -c:a copy \
			/promo/parts/09-sound.mp4
	else
		make_card 09-sound 7.5
	fi
else
	make_card 09-sound 7.5
fi

# 6. remake 段落也正規化，確保 fps 與像素格式一致。
for clip in /promo/remake/clips/seg*.mp4; do
	name=\$(basename \"\$clip\" .mp4)
	ffmpeg -v error -y -i \"\$clip\" \
		-f lavfi -i anullsrc=channel_layout=stereo:sample_rate=44100 -shortest \
		-vf \"scale=$OUT_WIDTH:$OUT_HEIGHT:flags=neighbor,fps=30,format=yuv420p\" \
		-c:v libx264 -crf $OUT_CRF -preset $OUT_PRESET -c:a aac -b:a 128k \
		\"/promo/parts/\$name.mp4\"
done

# 7. 依順序接起來。每一段實機畫面前面都有一張說明卡，看的人才知道在看什麼。
cd /promo/parts
: >concat.txt
for part in 01-title 02-what-is-this seg1 03-world-data seg2 05-systems seg3 seg4 seg5 \
            07-restored seg7 seg8 seg9 04-tiles-palette 08-original-tiles seg10 \
            09-sound 06-presentation seg6 13-platform-looks seg11 seg12 14-atari-st-interface seg13 \
            10-side-by-side original-clip \
            11-side-by-side-2 12-closing; do
	# seg10 要玩家自己的磁碟，沒錄到就跳過。
	if test -f \"\$part.mp4\"; then printf \"file '%s.mp4'\\n\" \"\$part\" >>concat.txt; fi
done
ffmpeg -v error -y -f concat -safe 0 -i concat.txt -c copy /promo/sundog-promo.mp4

# 8. 一支無聲 GIF，取切換主題那一段。
ffmpeg -v error -y -i /promo/parts/seg6.mp4 -t 18 \
	-vf 'fps=12,scale=960:-1:flags=neighbor,split[a][b];[a]palettegen[p];[b][p]paletteuse' \
	/promo/sundog-promo.gif

printf 'MP4 長度 '
ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 /promo/sundog-promo.mp4
"

printf '影片已產出：%s/sundog-promo.mp4 與 %s/sundog-promo.gif\n' "$promo_dir" "$promo_dir"
printf '含原版畫面，不得進版控或 release。\n'
