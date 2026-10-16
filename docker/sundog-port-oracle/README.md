# SunDog 公開移植 oracle 建置工具鏈

此 image 只提供建置／執行公開 `laanwj/sundog` Atari ST port 所需的工具。它不包含原始
SunDog binary、disk image、ROM 或本專案的 runtime code；來源與遊戲磁碟都在一次性容器
中唯讀或暫存掛載。

## 固定版本

- 基底：`debian:bookworm@sha256:813017f3d62be4b5891a7acca6a01bdcd4b8513daa81b1ab99d3a50385b26931`
- 公開移植專案：`https://github.com/laanwj/sundog.git`
- 研究 commit：`e56d8e2532134f0741a7f82b80f9357776779f27`（2026-08-11 查得 `main`）
- 已建置 image：`sundog-port-oracle:20260811`，image ID
  `sha256:64184d564cf943c1fec2264fcce09fb41bc0d1665b205aac7878a36d6c5617bb`
- 工具版本以 image 建置時的 Debian Bookworm 套件輸出與 image digest 為準，必須記錄於
  `RESEARCH-LOG.md`；不使用主機的 compiler、SDL 或 GUI。

## 建置

```sh
docker build --network=host \
  -t sundog-port-oracle:20260811 docker/sundog-port-oracle
```

## 使用邊界

- 只把使用者提供的 `[a2]` `.st` image 以唯讀方式提供給 oracle；不要把原始 disk 或
  upstream source commit 複製進本 repository。
- upstream port 依自己的 MIT license 發布；本專案只把它當交叉驗證工具，不複製其程式碼。
- `Xvfb`、SDL、截圖與測試都在有界的 `docker run --rm` 容器中執行；背景 `Xvfb` 必須由
  同一個 shell 的 `trap` 清理。
- 這個 port 直接解讀 Atari ST 的 p-system／自訂 disk layout，不需要 Atari TOS ROM；它
  因而可以補足 Hatari 缺少 TOS 的 environment blocker，但不能取代 Apple II oracle。
