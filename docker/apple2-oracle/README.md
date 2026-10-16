# Apple II oracle Docker image

用途：在隔離 Docker 中啟動 MAME Apple II driver，唯讀掛載 SunDog `.DSK`，取得 boot log、
畫面與可重播輸入。它是原版行為 oracle 工具，不是 remake runtime 的依賴。

## 固定基線

- base：`debian:bookworm`
- base digest：`sha256:813017f3d62be4b5891a7acca6a01bdcd4b8513daa81b1ab99d3a50385b26931`
- MAME：`0.251+dfsg.1-1`
- 建議 image tag：`sundog-apple2-oracle:20260811`

## 建置與基本檢查

```sh
docker build --network host \
  --tag sundog-apple2-oracle:20260811 \
  /home/anr2/cht/sundog/docker/apple2-oracle

docker run --rm --network none --memory=1g --cpus=2 --pids-limit=128 \
sundog-apple2-oracle:20260811 -version
```

已驗證的 driver 查詢：

```sh
docker run --rm --network none --memory=1g --cpus=2 --pids-limit=128 \
  sundog-apple2-oracle:20260811 -listfull
```

目前 image 的 MAME 版本是 `0.251`，可列出 `apple2`、`apple2e` 等 driver。Debian
套件本身**不附** Apple II 系統 ROM（`342-0133-a.chr`、`342-0135-b.64`、
`342-0134-a.64`、`342-0132-c.e12`、`341-0027-a.p5`、`341-0028-a.rom` 等），
少了它們 `apple2e` 會直接停住。

ROM 已依使用者授權另行取得，**只下載到 `/tmp/sundog-apple2-roms-20260811`，不進
repository 也不進封包**，以 `-rompath` 掛進容器；`apple2e -verifyroms` 回報
`romset apple2e is good`。完整命令與雜湊見
[`../../docs/apple2/mame-oracle-evidence.json`](../../docs/apple2/mame-oracle-evidence.json)。

判讀原則仍然成立：缺 ROM 造成的無畫面要記成環境問題，不能記成 DSK boot 失敗或
遊戲檔案失敗，也不能拿來路不明的 ROM 當成自動修復。

## 唯讀 DSK 掛載原則

```sh
docker run --rm --network none --memory=1g --cpus=2 --pids-limit=128 \
  --user "$(id -u):$(id -g)" \
  -v /home/anr2/cht/sundog/SUNDOG1.DSK:/roms/SUNDOG1.DSK:ro \
  -v /tmp/sundog-apple2-oracle-output:/output:rw \
  sundog-apple2-oracle:20260811 apple2e \
  -flop1 /roms/SUNDOG1.DSK -nomouse -sleep -aviwrite /output/oracle.avi
```

實際 MAME driver／選項需先以 `-listxml` 與一次性 smoke 試驗核對；不要把上述候選命令當成
已驗證的 SunDog 啟動流程。所有截圖／AVI／log 只能寫到明確 output 目錄，原始 image 永遠
唯讀。
