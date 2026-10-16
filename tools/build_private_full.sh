#!/usr/bin/env bash
set -euo pipefail

# 把三平台 release、玩家自備的原版磁碟與由該磁碟匯出的音效打成一份私人完整版 ZIP。
# 產物含原版磁碟與音效，只留本機／私人交付：不進版控、不進 GitHub Release。
#
# 用法：tools/build_private_full.sh /絕對/release目錄 /絕對/sundog.st /絕對/輸出目錄 版本
#
# 成本（實測）：音效匯出與封裝合計約 1 分鐘，單核。

release_dir=${1:?用法：build_private_full.sh <release目錄> <sundog.st> <輸出目錄> <版本>}
disk=${2:?}
out_root=${3:?}
version=${4:?}
if [[ ! "$version" =~ ^v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}$ ]]; then
	echo "版號不符合 v.<主>.<次>.<修訂>-YYYYMMDD：$version" >&2
	exit 2
fi
for path in "$release_dir" "$out_root"; do
	[[ "$path" == /* ]] || { echo "路徑必須是絕對路徑：$path" >&2; exit 2; }
done
test -f "$disk"
if [[ -e "$out_root/SunDog-$version-private-full" || -e "$out_root/SunDog-$version-private-full.zip" ]]; then
	echo "此版私人完整版已存在，禁止覆寫：$out_root" >&2
	exit 2
fi

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BUILD_IMAGE=${SUNDOG_BUILD_IMAGE:-sundog-remake-build:go1.25.12-bookworm-20261014-r1}
CACHE_DIR=${SUNDOG_GO_CACHE_DIR:-/tmp/sundog-release-gocache}
mkdir -p "$CACHE_DIR/build" "$CACHE_DIR/mod" "$out_root"

docker run --rm --network none \
	--memory 4g --cpus 2 --pids-limit 512 \
	--log-opt max-size=10m --log-opt max-file=3 \
	-u "$(id -u):$(id -g)" -e HOME=/tmp -e SUNDOG_VERSION="$version" \
	-v "$repo":/work:ro -v "$CACHE_DIR/build":/cache/build \
	-v "$CACHE_DIR/mod":/cache/mod:ro \
	-v "$release_dir":/release:ro -v "$(cd "$(dirname "$disk")" && pwd)":/disk:ro \
	-v "$out_root":/out \
	-w /work --entrypoint timeout "$BUILD_IMAGE" 180 bash -c "
set -eu
for output_path in /cache/build /out; do
 if [ -n \"\$(find \"\$output_path\" ! -uid \"\$(id -u)\" -print -quit)\" ]; then
  echo '輸出或快取含非目前使用者擁有的檔案，停止寫入' >&2
  exit 2
 fi
done
export PATH=/usr/local/go/bin:\$PATH GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
stage=/out/SunDog-\$SUNDOG_VERSION-private-full
test ! -e \"\$stage\"
mkdir -p \"\$stage/audio\"
cp /release/SunDog-\$SUNDOG_VERSION-linux-x86_64.AppImage \"\$stage/\"
cp /release/SunDog-\$SUNDOG_VERSION-macos-universal.zip \"\$stage/\"
cp /release/SunDog-\$SUNDOG_VERSION-windows-amd64.zip \"\$stage/\"
cp '/disk/$(basename "$disk")' \"\$stage/sundog.st\"
go run ./cmd/sundog-sound-export -disk \"\$stage/sundog.st\" -out \"\$stage/audio\" >/out/sound-export.log 2>&1 || {
 cat /out/sound-export.log >&2
 exit 1
}
python3 /work/tools/pack_private_full.py --stage \"\$stage\" --version \"\$SUNDOG_VERSION\" --output /out
"

printf '私人完整版已產出：%s\n' "$out_root"
ls -la "$out_root"
