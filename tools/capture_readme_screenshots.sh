#!/usr/bin/env bash
set -euo pipefail

# Docker-only normal Atari ST player captures. See docs/screenshots/README.md.
# The Apple II HD / direct-entry gallery retired on 2026-10-14.
test -f /.dockerenv || { echo '此工具只能在 Docker 容器內執行。' >&2; exit 2; }
out=${1:?請提供全新的 workplace 截圖輸出目錄}
: "${SUNDOG_ATARI_DISK:?請指定玩家自備的合法 .st 磁碟}"
test -f "$SUNDOG_ATARI_DISK"
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"
export GOPROXY=off GOTOOLCHAIN=local
export GOCACHE=${GOCACHE:-/tmp/sundog-readme-gocache}
binary=${SUNDOG_BINARY:-/tmp/sundog-readme-capture}
if [[ -z ${SUNDOG_BINARY:-} ]]; then
    go build -buildvcs=false -o "$binary" ./cmd/sundog-ebiten
fi
exec python3 tools/capture_atari_readme.py --binary "$binary" --disk "$SUNDOG_ATARI_DISK" --output "$out"
