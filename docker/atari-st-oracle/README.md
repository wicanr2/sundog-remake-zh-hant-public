# Atari ST oracle Docker image

用途：在隔離 Docker 中使用 Hatari／EmuTOS 盤點 Atari ST `.st`／`.stx` 的啟動與畫面路徑。
它是原版行為 oracle 工具，不是 remake runtime 依賴；所有原始磁碟都以唯讀方式掛載。

## 固定基線

- base：`debian:bookworm`
- base digest：`sha256:813017f3d62be4b5891a7acca6a01bdcd4b8513daa81b1ab99d3a50385b26931`
- Hatari：`2.4.1+dfsg-2`
- Xvfb：`2:21.1.7-3+deb12u12`
- Python：`3.11.2-1+b1`，只用於從使用者提供的外層 archive 取出 nested image
- xdotool：`1:3.20160805.1-5`，只用於將可重播的左鍵／座標輸入送到 Xvfb 內的 Hatari 視窗
- 建議 image tag：`sundog-atari-st-oracle:20260812`；`20260811` 保留作早期無 xdotool 證據的
  可回查基線

## 建置與 smoke

```sh
docker build --network host \
  --tag sundog-atari-st-oracle:20260812 \
  /home/anr2/cht/sundog/docker/atari-st-oracle

docker run --rm --network none --memory=1g --cpus=2 --pids-limit=128 \
  --entrypoint /bin/sh sundog-atari-st-oracle:20260812 -c \
  'hatari --help 2>&1 | grep -q "Usage:" && xdotool --version'
```

Debian 的 Hatari 套件即使印出完整的 `--help` 也回傳非零；因此 smoke 以 `Usage:` 輸出與
`xdotool --version` 的成功碼作 gate，而不是直接相信 Hatari CLI 的 exit status。

目前 Debian `hatari` 套件 image 沒有安裝 TOS／EmuTOS ROM（`/usr/share/hatari` 只有工具與
文件）；若要進入模擬，需另有合法、可回查的 ROM 或建立自由的 EmuTOS 工具鏈。若要宣稱
原版 TOS／硬體行為，仍需另外驗證。缺 TOS、SDL／Xvfb 或 driver 的退出，都要與遊戲檔案
錯誤分開記錄。

## xdotool 輸入邊界

`xdotool` 只對 Xvfb 中可辨識的 `Hatari v2.4.1` **child window** 發送真實 X11 滑鼠事件；
不得誤選 800×600 的 Xvfb root window，也不得把成功執行命令等同於遊戲接受輸入。原始
`[a2]` normal desktop 路徑、stable graphical program output 與一次中心左鍵未轉場的
負向結果分別見 [`../../docs/atari-st/hatari-normal-desktop-player-path-evidence.json`](../../docs/atari-st/hatari-normal-desktop-player-path-evidence.json)
與 [`../../docs/atari-st/hatari-title-input-probe-evidence.json`](../../docs/atari-st/hatari-title-input-probe-evidence.json)。

若輸入後 screenshot 未變，只能標為「此座標未取得可見轉場」；不能推論標題畫面沒有按鈕、
遊戲未接收鼠標，或可用 secondary port 的座標直接填補原始 binary。

## nested STX 的唯讀啟動骨架

以下命令只示範工具鏈邊界；它尚未是已驗證的 SunDog 正常玩家路徑。輸入 archive 只讀，
輸出截圖／log 寫入明確的工作目錄；Xvfb 與 Hatari 必須由同一個有界的 shell 擁有。

```sh
docker run --rm --network none --memory=1g --cpus=2 --pids-limit=128 \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e XDG_RUNTIME_DIR=/tmp/runtime \
  -v /home/anr2/cht/sundog:/workspace:ro \
  -v /tmp/sundog-atari-output:/output:rw \
  --entrypoint /bin/sh sundog-atari-st-oracle:20260812 -lc '
    mkdir -p /tmp/runtime /tmp/input /output
    python3 - <<"PY"
import io
import zipfile
outer = "/workspace/Sundog-Frozen-Legacy_Atari-ST_EN.zip"
with zipfile.ZipFile(outer) as source:
    for info in source.infolist():
        if not info.filename.lower().endswith(".zip"):
            continue
        with zipfile.ZipFile(io.BytesIO(source.read(info))) as nested:
            for member in nested.infolist():
                if member.filename.lower().endswith(".stx"):
                    open("/tmp/input/Sundog.stx", "wb").write(nested.read(member))
                    raise SystemExit
raise SystemExit("STX member not found")
PY
    Xvfb :99 -screen 0 640x480x24 -nolisten tcp & xvfb_pid=$!
    trap "kill $xvfb_pid 2>/dev/null || true" EXIT
    export DISPLAY=:99
    timeout 20s /usr/bin/hatari --machine st --confirm-quit off \
      --fast-forward on --disk-a /tmp/input/Sundog.stx \
      --screenshot-dir /output
  '
```

啟動前先用 `--version`、`--help` 與一次最小 smoke 核對 Hatari 版本／參數；不要把這段
骨架的畫面當成已完成的 STX sector、tileset 或遊戲規則證據。
