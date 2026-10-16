#!/bin/sh
# 取得 EmuTOS 開機 ROM（自由授權，GPLv2），驗雜湊後放進工作目錄。
# 用法：./fetch-emutos.sh <工作目錄>
# 產物：<工作目錄>/emutos192us.img，供 boot-emutos.sh 的 --tos 使用。
set -e
OUT="${1:?用法：fetch-emutos.sh <工作目錄>}"
ZIP_SHA=59abac06a2d29b0864c5a7cfb2af65f022c337aed34188e174a9a08cc737e4bc
IMG_SHA=8fbbf8b44fc3e34281eaf8cda5265510e9af9ccda0e3e409111648060d244cfc
URL=https://sourceforge.net/projects/emutos/files/emutos/1.4/emutos-192k-1.4.zip/download

mkdir -p "$OUT"
if [ -f "$OUT/emutos192us.img" ] &&
   [ "$(sha256sum "$OUT/emutos192us.img" | cut -d' ' -f1)" = "$IMG_SHA" ]; then
  echo "已有 emutos192us.img，雜湊相符，略過下載"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
# 下載是唯一需要網路的一步，跑在一次性容器裡
docker run --rm -u "$(id -u):$(id -g)" --network bridge --memory 512m --pids-limit 64 \
  --log-opt max-size=10m --log-opt max-file=3 \
  -v "$TMP":/w -w /w --entrypoint /bin/sh u5cht/dev:latest \
  -c "curl -sL -o emutos.zip '$URL'"

got="$(sha256sum "$TMP/emutos.zip" | cut -d' ' -f1)"
[ "$got" = "$ZIP_SHA" ] || { echo "ZIP 雜湊不符：$got"; exit 1; }
unzip -o -q -j "$TMP/emutos.zip" 'emutos-192k-1.4/etos192us.img' -d "$TMP"
got="$(sha256sum "$TMP/etos192us.img" | cut -d' ' -f1)"
[ "$got" = "$IMG_SHA" ] || { echo "ROM 雜湊不符：$got"; exit 1; }
cp "$TMP/etos192us.img" "$OUT/emutos192us.img"
echo "已取得 $OUT/emutos192us.img（EmuTOS 1.4 US，$IMG_SHA）"
