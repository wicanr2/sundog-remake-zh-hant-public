#!/usr/bin/env bash
set -euo pipefail

# 把同一份原始碼交叉編成 Linux 與 Windows 的 sundog-preview，各畫一次同一個場景，
# 再逐位元比。驗的是繪圖管線的輸出在兩個平台之間有沒有差，不是實機驗收（WORKLIST D1）。
#
# 用法：tools/cross_platform_render_parity.sh <工作目錄>
#
# 工作目錄會放 binary、go build cache 與 PNG；不寫進 repository。
# module cache 用主機的 ~/go/pkg/mod（唯讀掛載），所以建置容器不需要網路。

work=${1:?用法：cross_platform_render_parity.sh <工作目錄>}
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BUILD_IMAGE=${SUNDOG_BUILD_IMAGE:-sundog-remake-build:20260812}
WINE_IMAGE=${SUNDOG_WINE_IMAGE:-coab-wine-smoke:ubuntu-noble-20260826}
PIL_IMAGE=${SUNDOG_PIL_IMAGE:-mm2-pil:pillow11}
uidgid="$(id -u):$(id -g)"
common=(--rm --network none -u "$uidgid" --log-opt max-size=10m --log-opt max-file=3)

mkdir -p "$work"/{bin,gocache,shots,wineprefix}

echo "== 交叉建置 preview（Linux 與 Windows）"
docker run "${common[@]}" --memory 6g --cpus 4 --pids-limit 512 \
	-v "$repo":/src:ro -v "$HOME/go/pkg/mod":/gomod:ro \
	-v "$work/gocache":/cache -v "$work/bin":/out \
	-w /src --entrypoint bash "$BUILD_IMAGE" -c '
set -eu
export PATH=/usr/local/go/bin:$PATH GOCACHE=/cache GOMODCACHE=/gomod GOFLAGS="-buildvcs=false -mod=mod"
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -o /out/preview-linux ./cmd/sundog-preview
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o /out/preview.exe   ./cmd/sundog-preview
'

echo "== Linux 跑兩次（先證明場景可重現）"
for n in 1 2; do
	docker run "${common[@]}" --memory 2g --pids-limit 128 \
		-v "$repo":/src:ro -v "$work":/w -w /src \
		--entrypoint /w/bin/preview-linux "$BUILD_IMAGE" \
		-space-scene -out "/w/shots/space-linux$n.png" >/dev/null
done

echo "== wine 跑一次 Windows 版"
docker run "${common[@]}" --memory 2g --pids-limit 256 \
	-v "$work":/w -w /w -e WINEDEBUG=-all -e WINEPREFIX=/w/wineprefix -e HOME=/w \
	--entrypoint bash "$WINE_IMAGE" -c '
export PATH=/usr/lib/wine:$PATH
wine64 wineboot -i >/dev/null 2>&1 || true
wine64 /w/bin/preview.exe -space-scene -out "Z:\w\shots\space-win.png"' >/dev/null

echo "== 比對"
docker run "${common[@]}" --memory 2g --pids-limit 128 \
	-v "$repo/tools":/t:ro -v "$work/shots":/s -w /t \
	--entrypoint python3 "$PIL_IMAGE" check_render_parity.py \
	/s/space-linux1.png /s/space-win.png \
	--same-platform /s/space-linux1.png /s/space-linux2.png
