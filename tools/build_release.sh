#!/usr/bin/env bash
set -euo pipefail

# 打包三個平台的可執行套件到 dist-all。
# 規格：docs/release.md
#
#   linux   → AppImage（x86_64）
#   windows → zip（amd64，檔名 UTF-8 旗標開啟）
#   macos   → .app bundle 的 zip（universal：x86_64 + arm64，osxcross 交叉編）
#
# 用法：tools/build_release.sh /絕對/輸出目錄 [版本]
#
# 需要的外部檔（容器內沒有網路，先在主機備好）：
#   $SUNDOG_APPIMAGE_DIR/appimagetool.AppImage
#   $SUNDOG_APPIMAGE_DIR/runtime-x86_64
#
# 成本（實測）：三個平台合計約 3 分鐘（module cache 已暖）。

out_root=${1:?用法：build_release.sh /絕對/輸出目錄 [版本]}
if [[ "$out_root" != /* ]]; then
	echo "輸出目錄必須是絕對路徑。" >&2
	exit 2
fi

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version=${2:?必須明確提供完整版本，例如 v.1.0.1-20260901}
if [[ ! "$version" =~ ^v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}$ ]]; then
	echo "版號不符合 v.<主>.<次>.<修訂>-YYYYMMDD：$version" >&2
	exit 2
fi

BUILD_IMAGE=${SUNDOG_BUILD_IMAGE:-sundog-remake-build:go1.25.12-bookworm-20261014-r1}
OSXCROSS_IMAGE=${SUNDOG_OSXCROSS_IMAGE:-sundog-remake-osxcross:go1.25.12-sdk15.5-20261014-r1}
# 封裝要用有 `file` 的 image——appimagetool 缺它會直接拒跑。圖示已經是版控裡的檔案，
# 所以這一步不需要 PIL。
PACKAGE_IMAGE=${SUNDOG_PACKAGE_IMAGE:-$BUILD_IMAGE}
CACHE_DIR=${SUNDOG_GO_CACHE_DIR:-/tmp/sundog-release-cache-v2-$(id -u)}
APPIMAGE_DIR=${SUNDOG_APPIMAGE_DIR:?需要 SUNDOG_APPIMAGE_DIR，裡面要有 appimagetool.AppImage 與 runtime-x86_64}

test -f "$APPIMAGE_DIR/appimagetool.AppImage"
test -f "$APPIMAGE_DIR/runtime-x86_64"
if compgen -G "$out_root/SunDog-$version-*" >/dev/null; then
	echo "此版套件已存在，請使用新的輸出目錄，禁止覆寫：$out_root" >&2
	exit 2
fi
mkdir -p "$CACHE_DIR/build" "$CACHE_DIR/mod"

stage="$out_root/staging"
if [[ -e "$stage" ]]; then
	echo "暫存目錄已存在，請使用新的輸出目錄以保留先前產物：$stage" >&2
	exit 2
fi
mkdir -p "$stage/bin" "$out_root"

# 使用建置映像內已鎖版的模組，避免 macOS 映像缺模組時偷偷依賴網路。
# 寫入前檢查整個輸出／快取的擁有權；歷史 root-owned 產物必須另外處理。
docker run --rm --network none --memory 512m --cpus 1 --pids-limit 64 \
	-u "$(id -u):$(id -g)" \
	-v "$repo":/work:ro -v "$CACHE_DIR":/cache -v "$out_root":/out \
	-w /work --entrypoint timeout "$BUILD_IMAGE" 120 bash -c '
set -euo pipefail
for path in /cache /out; do
	if [[ -n "$(find "$path" ! -uid "$(id -u)" -print -quit)" ]]; then
		echo "輸出含非目前使用者擁有的檔案，停止寫入：$path" >&2
		exit 2
	fi
done
if [[ -z "$(ls -A /cache/mod)" ]]; then
	cp -R "$(go env GOMODCACHE)/." /cache/mod/
fi
GOMODCACHE=/cache/mod GOPROXY=off go mod verify
'

run_build() {
	docker run --rm --network none \
		--memory 4g --cpus 2 --pids-limit 512 \
		--log-opt max-size=10m --log-opt max-file=3 \
		-u "$(id -u):$(id -g)" -e HOME=/tmp -e SUNDOG_VERSION="$version" \
		-v "$repo":/work:ro -v "$CACHE_DIR/build":/cache/build \
		-v "$CACHE_DIR/mod":/cache/mod:ro -v "$stage":/stage -e GOPROXY=off \
		-w /work --entrypoint timeout "$1" 600 bash -c "$2"
}

echo "== 版本 $version"

# ---- Linux（cgo，因為 Ebiten 的 X11／OpenGL 後端要它）----
echo "== linux/amd64"
run_build "$BUILD_IMAGE" '
set -eu
export PATH=/usr/local/go/bin:$PATH GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.buildVersion=$SUNDOG_VERSION" \
	-o /stage/bin/sundog-remake-linux-amd64 ./cmd/sundog-ebiten
# 音效轉檔工具沒有畫面，不用 cgo。
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
	-o /stage/bin/sundog-sound-export-linux-amd64 ./cmd/sundog-sound-export
'

# ---- Windows（純 Go，沒有 cgo，也沒有要隨包散布的 DLL）----
echo "== windows/amd64"
run_build "$BUILD_IMAGE" '
set -eu
export PATH=/usr/local/go/bin:$PATH GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui -X main.buildVersion=$SUNDOG_VERSION" \
	-o /stage/bin/sundog-remake.exe ./cmd/sundog-ebiten
# 轉檔工具是命令列的，要留著主控台視窗才看得到輸出。
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
	-o /stage/bin/sundog-sound-export.exe ./cmd/sundog-sound-export
'

# ---- macOS（cgo 走 osxcross，兩個弧各編一次再 lipo）----
echo "== darwin universal"
run_build "$OSXCROSS_IMAGE" '
set -eu
export PATH=/osxcross/bin:/usr/local/go/bin:$PATH GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
eval "$(osxcross-conf)"
lipo_tool=x86_64-apple-$OSXCROSS_TARGET-lipo
for a in arm64 amd64; do
	cc=$([ $a = arm64 ] && echo arm64-apple-$OSXCROSS_TARGET-clang || echo x86_64-apple-$OSXCROSS_TARGET-clang)
	CGO_ENABLED=1 GOOS=darwin GOARCH=$a CC=$cc CXX=${cc}++ \
		go build -trimpath -ldflags "-s -w -X main.buildVersion=$SUNDOG_VERSION" -o /stage/bin/sundog-darwin-$a ./cmd/sundog-ebiten
	CGO_ENABLED=0 GOOS=darwin GOARCH=$a \
		go build -trimpath -ldflags "-s -w" -o /stage/bin/sound-export-$a ./cmd/sundog-sound-export
done
$lipo_tool -create /stage/bin/sundog-darwin-arm64 /stage/bin/sundog-darwin-amd64 \
	-output /stage/bin/sundog-remake-macos-universal
$lipo_tool -create /stage/bin/sound-export-arm64 /stage/bin/sound-export-amd64 \
	-output /stage/bin/sundog-sound-export-macos-universal
rm -f /stage/bin/sundog-darwin-arm64 /stage/bin/sundog-darwin-amd64 \
	/stage/bin/sound-export-arm64 /stage/bin/sound-export-amd64
# 靜態驗收：兩個弧都在、arm64 有 ad-hoc 簽章（沒有的話 Apple Silicon 會直接 Killed: 9）、
# 相依只在 /usr/lib 與 /System/Library。
$lipo_tool -info /stage/bin/sundog-remake-macos-universal | grep -q "x86_64 arm64"
$lipo_tool -thin arm64 /stage/bin/sundog-remake-macos-universal -output /tmp/thin-arm64
x86_64-apple-$OSXCROSS_TARGET-otool -l /tmp/thin-arm64 | grep -q LC_CODE_SIGNATURE
for a in arm64 x86_64; do
	$lipo_tool -thin $a /stage/bin/sundog-remake-macos-universal -output /tmp/thin-$a
	if x86_64-apple-$OSXCROSS_TARGET-otool -L /tmp/thin-$a | tail -n +2 |
		grep -vE "^\s+(/usr/lib/|/System/Library/)"; then
		echo "macOS binary 連到了玩家機器上不存在的東西" >&2
		exit 1
	fi
done
'

# ---- 共用酬載與三種封裝 ----
echo "== 封裝"
docker run --rm --network none \
	--memory 4g --cpus 2 --pids-limit 512 \
	--log-opt max-size=10m --log-opt max-file=3 \
	-u "$(id -u):$(id -g)" -e HOME=/tmp \
	-v "$repo":/work:ro -v "$stage":/stage -v "$out_root":/out \
	-v "$APPIMAGE_DIR":/appimage:ro \
	-w /work --entrypoint timeout "$PACKAGE_IMAGE" 300 bash -c "
set -eu
export SUNDOG_VERSION='$version'
python3 /work/tools/package_release.py --repo /work --staging /stage --output /out --version '$version'
"

rm -rf "$stage"
printf '套件已產出：%s\n' "$out_root"
ls -la "$out_root"
