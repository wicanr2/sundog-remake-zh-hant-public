# Docker 建置與測試

需求為 Docker、Git 與有桌面環境的目標電腦。專案使用 Go 1.25.12；依賴版本由 go.mod 與 go.sum 固定。

先在儲存庫根目錄建置鎖定的 Linux 工具鏈。這一步需要網路下載映像、Debian 套件與 Go 模組：

```sh
docker build --memory 4g --cpu-period 100000 --cpu-quota 200000 \
  -f docker/sundog-remake-build/Dockerfile \
  -t sundog-remake-build:go1.25.12-bookworm-20261014-r1 .
```

建立可寫輸出目錄，再於容器中編譯。來源唯讀掛載，不需要原版磁碟：

```sh
mkdir -p workplace/public-build
docker run --rm --network none --memory 4g --cpus 2 --pids-limit 512 \
  --user "$(id -u):$(id -g)" -e HOME=/tmp -e GOCACHE=/tmp/go-build \
  -e GOPROXY=off -e GOMODCACHE=/go/pkg/mod \
  -v "$PWD":/src:ro -v "$PWD/workplace/public-build":/out \
  -w /src --entrypoint timeout \
  sundog-remake-build:go1.25.12-bookworm-20261014-r1 600 \
  go build -trimpath -buildvcs=false -o /out/sundog-remake ./cmd/sundog-ebiten
```

Linux 玩家在桌面電腦的儲存庫根目錄執行 `./workplace/public-build/sundog-remake`，依選碟畫面指定合法 `sundog.st`。遊戲須能讀取隨來源提供的字型與翻譯目錄。受 Docker-only 規約管理的開發與驗證環境，遊戲也須在已配置桌面或 Xvfb 的容器內執行。

普通測試不需要原版磁碟。GPU 套件需 Xvfb：

```sh
docker run --rm --network none --memory 4g --cpus 2 --pids-limit 512 \
  --user "$(id -u):$(id -g)" -e HOME=/tmp -e GOCACHE=/tmp/go-build \
  -e GOPROXY=off -e GOMODCACHE=/go/pkg/mod -e LIBGL_ALWAYS_SOFTWARE=1 \
  -v "$PWD":/src:ro -w /src --entrypoint timeout \
  sundog-remake-build:go1.25.12-bookworm-20261014-r1 600 \
  bash -c 'set -e
    Xvfb :91 -screen 0 1600x1000x24 -nolisten tcp &
    display_pid=$!
    trap "kill $display_pid 2>/dev/null || true; wait $display_pid 2>/dev/null || true" EXIT
    export DISPLAY=:91
    sleep 1
    go test ./cmd/... ./internal/...'
```

同一容器環境可執行 `go vet ./cmd/... ./internal/...` 與 `(cd tools && python3 -m unittest test_package_public test_package_release)`。需要 `HD_WORLD_DISK` 的來源路由檢查、受控原版匯出及正常玩家自動化須另備合法磁碟或研究環境；普通測試通過不等於這些項目已重驗。

三平台封裝入口為 `tools/build_release.sh`，需另外準備 AppImage 工具與 macOS SDK 工具鏈。公開 repo 不附 SDK、原版資料、既有二進位 Release 或本機完整版。
