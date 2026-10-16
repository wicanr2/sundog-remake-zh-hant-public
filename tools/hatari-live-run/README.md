# 原版 Hatari 實跑驅動（D4 對拍用）

讓原版 SunDog 在 Hatari（容器 `sundog-atari-st-oracle:20260812`）裡實跑，
從外部注入滑鼠輸入、擷取畫面，供與重製版逐項對拍。

## 用法

```sh
# 取得開機 ROM（EmuTOS，自由授權；驗雜湊，已有就略過）
./fetch-emutos.sh <工作目錄>

# 全新開機（桌面 → 啟動 SUNDOG.PRG → 標題；約 3 分鐘）
cp boot-emutos.sh <工作目錄>/
docker run -d --name sundog-d4 --log-opt max-size=10m --log-opt max-file=3 \
  --network none --cpus 2 --memory 2g --pids-limit 256 --entrypoint /bin/sh \
  -v <原版磁碟目錄>:/in:ro -v <工作目錄>:/out \
  sundog-atari-st-oracle:20260812 /out/boot-emutos.sh
# 從記憶體快照續跑（約 15 秒；快照含原版位元組，不入 repository）
#   → boot-restore.sh，hatari --memstate <sav>
# 宿主端控制：
python3 drive.py snap      # 擷取一張（832×552）
python3 drive.py rel DX DY # 相對移動滑鼠
python3 drive.py click     # 左鍵單擊（drive.py 只做單擊；要長按得繞過它直接下 xdotool）
```

`/out` 需先放開機 ROM，`/in` 是含 `sundog-a2.st` 的唯讀目錄。ROM 有兩種：

- **EmuTOS**（`boot-emutos.sh`，預設）：`fetch-emutos.sh` 下載並驗雜湊，產出
  `emutos192us.img`。自由授權，不必持有版權 TOS，所以這條路隨時可重跑。
- **TOS 1.02**（`boot-fresh.sh`）：`tos102.img`，SHA-256 `8a5914bb…`，見
  `docs/atari-st/tos-and-hatari.md`。要自己準備。

兩者的桌面圖示座標不同（見下），其餘流程相同。EmuTOS 不是 SunDog 當年跑的 TOS，
所以「畫面是原版的」要靠畫面自己證明，不能靠 ROM 的身分——`tools/check_title_pixel_parity.py`
就是那道閘門，標題畫面未被覆蓋的 18,760 個像素與磁碟解出的影像逐像素相同。

**EmuTOS 這條路到標題畫面為止。** 遊戲收不到輸入：XTEST 與 cmd-fifo 兩條注入路徑、滑鼠與
鍵盤都送過，畫面只有自身動畫在動（任兩張截圖恆差 24,776 px）。成因與一手證據見
`docs/atari-st/tos-and-hatari.md`。要操作遊戲、走逐場景對拍，用 `boot-fresh.sh` 配 TOS 1.02。

## 走通的關鍵（來之不易，別重踩）

- **開機 ROM 挑得對才有畫面。** TOS 2.05 配 `--machine st` 是 bus error 黑畫面，
  配 `--machine megaste`／`ste` 則停在白畫面、PC 卡在 `$e0019e` 的開機期記憶體偵測，
  換 memsize、cpuclock、monitor、tos-res 都一樣（`--machine tt` 則被 Hatari 自己擋掉：
  `This TOS version does not work in TT/Falcon mode`）。能用的是 TOS 1.02 或 EmuTOS。
- **EmuTOS 的桌面圖示在最上一列**，TOS 1.02 的在畫面中段。沿用另一份的座標會點到
  選單列——`boot-emutos.sh` 的 `(161,144)`／`(203,261)` 與 `boot-fresh.sh` 的
  `(160,177)`／`(224,249)` 不能互換。
- **`xdotool` 不能帶 `--window`**：那走 XSendEvent，SDL 一律丟棄——先前
  「送滑鼠事件失敗」的根因。不帶 `--window` 走 XTEST 就能送達。
- **FTL 商標 splash 會停約 60–80 秒後自己轉場**到標題；15 秒內「畫面收斂」
  是誤判。
- `--cmd-fifo` 的 FIFO 要讓 hatari 自己建，先 `mkfifo` 它會退出。
- 控制 socket 的 `hatari-event` 沒有左鍵；左鍵用 XTEST，`doubleclick`／
  `rightdown` 走 fifo 也可。
- **座標映射**：832×552 截圖的 ST 內容區在 (96,58) 起、2× 縮放；
  桌面模式 host→截圖 約 `x×832/800、y+24`；點擊判定用的是**箭尖像素**，
  對不準差 10px 就會落空。
- **遊戲內的滑鼠是相對的，而且主機指標會頂到螢幕邊界**。`xdotool mousemove_relative`
  一旦把主機指標推到 `(0,0)`，同方向再送就沒有事件，ST 游標會**靜悄悄停住**，
  畫面上那個箭頭從此不是現在的位置。可靠的定位是三步：
  1. **歸零**——主機指標先甩到右下 `(795,585)`，再往左上推 `900×700`，兩邊同時貼邊，
     ST 游標停在 `(0,0)`；連做兩次歸零得到逐像素相同的畫面。
  2. **換算**——2.06（水平）／2.09（垂直）主機像素等於 1 ST 像素
     （量法：從原點送 400 主機像素得到 194 格、送 300 得到 143 格）。
  3. **回授**——往右下推 8 主機像素、前後兩張相減，取變動連通區塊的左上角即為推之前的
     箭尖位置；差多少再補多少。落點誤差 ≤4 px。
  不能用形狀比對找箭頭：它畫在黑底上是實心三角，畫在船艙圖上只剩外框。
- **關浮動視窗的右鍵要按在視窗裡面**，按在視窗外沒有作用。
- 拼名字畫面：**點字母格才會打字**（左鍵單擊那一格，字會以大寫出現在 `Name:` 後面），
  `+` 那一格完成。游標只是掃過去不會打字，鍵盤打字也沒有反應。
- 屬性畫面：440 點可以**完全不分配**——右鍵叫出 `Done`／`Adjust`，按 `Done` 就過
  （會先警告 `Don't waste!`）。要分配的話點屬性條選中，按住右端綠色上箭頭加點；按太久會把 440 點
  全灌進同一項。右鍵＝完成／離開。

## 開航段與遭遇（第九輪學到的）

- **航段要從 `SUB-LIGHT` 開，不是 `MAPS`。** MAPS 的三張圖是唯讀模式（`read_map` 第 4 個參數 0），
  點行星只給讀數。軌道 → NAVIGATE → SUB-LIGHT（第三列，ST 座標約 (40,67)）→ 系統圖上點行星白點
  兩下（5×5 實心圓；船 6×8、躍遷點 5×7；點到船只會 `Current location selected`）。
- **空船不會遇到海盜。** `update_movement` 要求貨艙 `Gec`（RAM `0x21540`／`0x21544`，word 0 的
  bits 0–4）裝著 2–30 的貨物；三份存檔都是 0。補貨後敵艦一分鐘內就到，而且會在你開戰術之前把船打爆
  ——船體在 `0x214de`（`G5b[0xc0]`），補成 255 再還原。
- **畫面從快照解，不要用截圖對變數。** `decode_vram.py <sav>` 從 RAM `0xf8000` 解出 320×200 那一幀
  （4 平面交錯），與同一份快照裡的變數是同一瞬間。`analyze_enc.py <sav>…` 對戰術畫面的五條斷言。
- **`savemem` 前先刪掉 `--memstate` 那個檔（就是啟動時給的那個路徑，不是 `~/.config/hatari/`；原檔先另存一份），然後輪詢 log。** 檔案還在會跳 stdin 的 `overwrite?`
  提示，整個模擬器卡住只能重開（重開容器要 `--entrypoint sh`，image 的 entrypoint 是 hatari 本身）；固定 sleep 不可靠（機器上另有容器時 4 秒不夠），要等
  `hatari-pilot.log` 裡 `Memory state file saved` 的行數增加。
- 戰術變數（`MAINLIB` 基底 `0x21368`，字編號 ×2）：`Gcf`／`Gd0` 兩軸角度、`Gdb` 距離、`Gd8` 艦型。
- **VRAM 種點法**：要量「某段程式擦掉哪塊」，把快照裡那一帶的空白像素點亮（plane 0 的 bit）再還原，
  第一幀裡消失的像素就是被塗掉的區域；比找一張剛好有星點落在框裡的畫面快得多。
  種點要另存一份，`savemem` 會把 `--memstate` 那個檔蓋掉。

## 星圖與地面的離線比對（第十輪）

- `star_map.py <sav>`：VRAM 解幀，找星系記號團塊，以 Woremed (176,64) 定原點；`sys_records.py <sav>`
  讀執行時的 12 筆星系記錄（`0x215a6` 起、每筆 10 words）。**對畫面一律用執行時座標**，出廠值
  經 `init_systems` 的間距重擲後可以差好幾格。
- `fwd_city.py <城市索引> <sav>`（要在掛了 repo 的容器裡跑：`-v repo:/work:ro -v 快照目錄:/out`，
  純 Python，不需要 numpy）：城市地圖 → 地形碼 → 圖塊 → 8×5 格的正向模擬，先掃捲動位置再逐格比。
  Drahew 正對照：(7,4)、36/40。
- **別改 `G4a` 的城市欄位來換城市**：城市地圖是 `init_city` 載進緩衝的，改索引只會讓 `draw_city_map`
  讀到全 0 的地圖（整張畫面變成背景圖樣），再走幾步就純色卡死。
- `LAND` 在行星圖模式下由 `get_dest` 自動選船所在的城市；要落在別的城市得先 `SUB-LIGHT` 到另一顆行星。

## 船艙站位（第十一輪）

站位就是視窗表編號 65–77 的矩形（`get_location` 對角色 sprite 做 `in_r_window`，跳過 67）：
65（x 60–64、y 90–101）＝駕駛台、66＝貨艙、**68（x 68–80、y 108–116）＝氣閘**——已降落時
走上去就出船到城市地面。從駕駛台按兩次右鍵回船艙，角色落在椅子上（視窗 65），游標放 (74,113)
按住兩秒就到。
