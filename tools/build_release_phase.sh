#!/usr/bin/env bash
set -euo pipefail
# Docker-native build phase. Entry and mounts: docs/release.md.
test -f /.dockerenv || { echo "只能在Docker執行" >&2; exit 2; }
phase=${1:?phase linux/windows/macos/package}
stage=${2:?existing staging directory}
version=${3:?complete version}
[[ "$version" =~ ^v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}$ ]] || exit 2
test -d "$stage"
[[ -z "$(find "$stage" ! -uid "$(id -u)" -print -quit)" ]] || exit 2
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"
export SUNDOG_VERSION="$version" PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local GOPROXY=off
if [[ "$phase" != package ]]; then
 export GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
 test -d "$GOCACHE"
 test -d "$GOMODCACHE"
 [[ "$(go env GOVERSION)" == go1.25.12 ]] || exit 2
 mkdir -p "$stage/bin"
fi
case "$phase" in
linux)
test ! -e "$stage/bin/sundog-remake-linux-amd64"
test ! -e "$stage/bin/sundog-sound-export-linux-amd64"

set -eu
export PATH=/usr/local/go/bin:$PATH GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.buildVersion=$SUNDOG_VERSION" \
	-o "$stage"/bin/sundog-remake-linux-amd64 ./cmd/sundog-ebiten
# 音效轉檔工具沒有畫面，不用 cgo。
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
	-o "$stage"/bin/sundog-sound-export-linux-amd64 ./cmd/sundog-sound-export
;;
windows)
test ! -e "$stage/bin/sundog-remake.exe"
test ! -e "$stage/bin/sundog-sound-export.exe"

set -eu
export PATH=/usr/local/go/bin:$PATH GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui -X main.buildVersion=$SUNDOG_VERSION" \
	-o "$stage"/bin/sundog-remake.exe ./cmd/sundog-ebiten
# 轉檔工具是命令列的，要留著主控台視窗才看得到輸出。
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
	-o "$stage"/bin/sundog-sound-export.exe ./cmd/sundog-sound-export
;;
macos)
test ! -e "$stage/bin/sundog-remake-macos-universal"
test ! -e "$stage/bin/sundog-sound-export-macos-universal"

set -eu
export PATH=/osxcross/bin:/usr/local/go/bin:$PATH GOCACHE=/cache/build GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
eval "$(osxcross-conf)"
lipo_tool=x86_64-apple-$OSXCROSS_TARGET-lipo
for a in arm64 amd64; do
	cc=$([ $a = arm64 ] && echo arm64-apple-$OSXCROSS_TARGET-clang || echo x86_64-apple-$OSXCROSS_TARGET-clang)
	CGO_ENABLED=1 GOOS=darwin GOARCH=$a CC=$cc CXX=${cc}++ \
		go build -trimpath -ldflags "-s -w -X main.buildVersion=$SUNDOG_VERSION" -o "$stage"/bin/sundog-darwin-$a ./cmd/sundog-ebiten
	CGO_ENABLED=0 GOOS=darwin GOARCH=$a \
		go build -trimpath -ldflags "-s -w" -o "$stage"/bin/sound-export-$a ./cmd/sundog-sound-export
done
$lipo_tool -create "$stage"/bin/sundog-darwin-arm64 "$stage"/bin/sundog-darwin-amd64 \
	-output "$stage"/bin/sundog-remake-macos-universal
$lipo_tool -create "$stage"/bin/sound-export-arm64 "$stage"/bin/sound-export-amd64 \
	-output "$stage"/bin/sundog-sound-export-macos-universal
rm -f "$stage"/bin/sundog-darwin-arm64 "$stage"/bin/sundog-darwin-amd64 \
	"$stage"/bin/sound-export-arm64 "$stage"/bin/sound-export-amd64
# 靜態驗收：兩個弧都在、arm64 有 ad-hoc 簽章（沒有的話 Apple Silicon 會直接 Killed: 9）、
# 相依只在 /usr/lib 與 /System/Library。
$lipo_tool -info "$stage"/bin/sundog-remake-macos-universal | grep -q "x86_64 arm64"
$lipo_tool -thin arm64 "$stage"/bin/sundog-remake-macos-universal -output /tmp/thin-arm64
x86_64-apple-$OSXCROSS_TARGET-otool -l /tmp/thin-arm64 | grep -q LC_CODE_SIGNATURE
for a in arm64 x86_64; do
	$lipo_tool -thin $a "$stage"/bin/sundog-remake-macos-universal -output /tmp/thin-$a
	if x86_64-apple-$OSXCROSS_TARGET-otool -L /tmp/thin-$a | tail -n +2 |
		grep -vE "^\s+(/usr/lib/|/System/Library/)"; then
		echo "macOS binary 連到了玩家機器上不存在的東西" >&2
		exit 1
	fi
done
;;
package)
 test -d /out
 [[ -z "$(find /out ! -uid "$(id -u)" -print -quit)" ]] || exit 2
 test -f /appimage/appimagetool.AppImage
 test -f /appimage/runtime-x86_64
 python3 "$repo/tools/package_release.py" --repo "$repo" --staging "$stage" --output /out --version "$version"
 ;;
*) echo "未知階段：$phase" >&2; exit 2 ;;
esac
