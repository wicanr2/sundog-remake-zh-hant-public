# 陽犬號 SunDog：Atari ST HD 重製版

[繁體中文](README.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [English](README.en.md)

以 Go 與 Ebitengine 重寫的科幻冒險遊戲。支援繁體中文、簡體中文、日文、韓文與英文，保留 Atari ST 操作分區，新增完整高解析重畫。

最新已發布版本為 **v.1.2.0-20261016**。本公開儲存庫提供引擎原始碼、工具、翻譯與獨立 HD 美術。遊戲需要玩家自備合法 Atari ST `sundog.st`；不附原版磁碟、圖像、音效或手冊掃描。原私人專案的 Git 歷史、對拍圖片與舊 Release 留在私人儲存庫。

![正常新局的獨立 HD 船艙](docs/screenshots/hd-shipboard.png)

## 故事背景

故事發生在 Jebal 帝國保護領內的 Drahew 區域。12 個星系、18 顆有人居住的行星之間，商船運送著城市與殖民地所需的物資。你將駕駛陽犬號穿梭其中，從太空港的交易所走進船艙，再飛往下一個陌生世界。

主角 Zed 原本在玻璃礦坑裡做苦工。素未謀面的叔叔 Brock Dor-Ceed 死後，留給他一艘星際貨船 SunDog，以及分散在各星系的銀行帳戶代碼。這艘船滿是修補與戰鬥痕跡，卻讓他第一次有機會離開礦坑。

遺產也包含叔叔的債務與未完成的殖民合約。你必須找到 Jondd 上的 Banville 殖民地，採購並交付建設物資，再把散布於各地倉庫的冷凍殖民者送到那裡。殖民地會分階段提出需求，你得靠貿易籌措燃料、零件與運費，讓這艘老船繼續航行。

旅途中，海盜可能逼你拋棄貨物，城市裡的搶匪也會盯上你的現金。每趟航行都要考量報價、航距、油量與船況；回到船上，還得維修設備、進食與休息。你的目標是讓殖民地成長，也讓自己保住這份得來不易的自由。

背景依《SunDog: Frozen Legacy》Atari ST 原版手冊第 3–4 頁序章與遊戲介紹改寫。手冊與掃描未隨本公開來源提供。Zed 是手冊主角，遊戲內可自行命名。

## 全套 Atari ST HD

HD 採「清晰精繪」方向，保留明亮色彩、船形與原分區。標題、船艙、三層導航、城市與地表、室內、人物、物品、工程、戰術、交易服務與終局均接入正式呈現。330 個圖塊、17 種室內底圖、31 種圖示與空槽使用獨立資產；碰撞、價格、時間與存檔規則沿既有引擎。

`F8` 依序切換 **Atari ST → Atari ST HD → Apple II → Atari ST**。預設仍是 Atari ST，選過的呈現會保存；也能在 `F2` 選項中選擇。原版呈現所需圖像於執行時讀取玩家磁碟。

| 城市與停泊船 | 星系導航 |
|---|---|
| ![HD 城市](docs/screenshots/hd-ground.png) | ![HD 星系圖](docs/screenshots/hd-system.png) |
| 工程維修 | 船上物資 |
| ![HD 工程](docs/screenshots/hd-engineering.png) | ![HD 物資](docs/screenshots/hd-stores.png) |

圖片為正式整合候選的正常玩家操作擷取，分屬不同分支，不宣稱同一時間狀態。僅展示獨立 HD 重畫，未包含原版對拍圖片。來源雜湊見[公開來源清單](PUBLIC-SOURCE-MANIFEST.json)。

## 五語 HD 遊玩截圖

同一個 Dramming 倉庫保存，經遊戲介面切換五種語言後擷取。點圖可看 1280×800 原圖。

<table>
<tr>
<td><a href="docs/screenshots/hd-warehouse-zh-Hant-20261016.png"><img src="docs/screenshots/hd-warehouse-zh-Hant-20261016.png" alt="繁體中文 · Atari ST HD · Dramming" width="416"></a><br>繁體中文</td>
<td><a href="docs/screenshots/hd-warehouse-zh-Hans-20261016.png"><img src="docs/screenshots/hd-warehouse-zh-Hans-20261016.png" alt="简体中文 · Atari ST HD · Dramming" width="416"></a><br>简体中文</td>
</tr>
<tr>
<td><a href="docs/screenshots/hd-warehouse-ja-20261016.png"><img src="docs/screenshots/hd-warehouse-ja-20261016.png" alt="日本語 · Atari ST HD · Dramming" width="416"></a><br>日本語</td>
<td><a href="docs/screenshots/hd-warehouse-ko-20261016.png"><img src="docs/screenshots/hd-warehouse-ko-20261016.png" alt="한국어 · Atari ST HD · Dramming" width="416"></a><br>한국어</td>
</tr>
<tr>
<td><a href="docs/screenshots/hd-warehouse-en-20261016.png"><img src="docs/screenshots/hd-warehouse-en-20261016.png" alt="English · Atari ST HD · Dramming" width="416"></a><br>English</td>
</tr>
</table>

畫面來自目前來源主線的正常保存載入，沒有後製。倉庫「交換」是 v.1.2.0-20261016 發布後新增的功能，尚未收入該版安裝包。來源與雜湊見[公開檔案清單](PUBLIC-SOURCE-MANIFEST.json)。

## 建置與執行

[下載 v.1.2.0-20261016 引擎](https://github.com/wicanr2/sundog-remake-zh-hant-public/releases/tag/v.1.2.0-20261016)，或依[Docker 建置說明](docs/build.md)自行編譯。啟動後選擇自己的合法 `sundog.st`。未匯入原版音效時保持靜音。

在 `F2` 選項的語言列以 `←`／`→` 切換五語，立即套用並記住設定。也能用 `-locale ja`、`-locale ko` 或 `-locale en` 啟動。語言不改遊戲狀態；翻譯與檢查方式見[語系說明](docs/i18n/README.md)。翻譯尚待母語編輯完整校閱。

| 平台 | 既有交付的驗證範圍 |
|---|---|
| Linux x86_64 | 正常新局、三呈現、保存重開 |
| Windows x64 | Wine 內正式腳本與保存重開；實機未驗 |
| macOS Intel／Apple Silicon | 雙架構與簽章靜態檢查；實機未驗 |

本公開來源另做乾淨建置與測試，結果見[目前狀態](CONTEXT.md)。不提供含原版資料的本機完整版或音效影片。

## 操作

| 操作 | 按鍵 |
|---|---|
| 說明／選項／靜音 | `F1`／`F2`／`F7` |
| 呈現／角色保存庫 | `F8`／`F5` |
| 返回／離開並保存 | `Esc` 或右鍵／`Q` 或 `F10` |
| 船艙功能 | `N` 導航、`X` 戰術、`E` 工程、`B` 物資、`C` 貨艙、`H` 狀態、`G` 下船 |
| 移動／確認 | 方向鍵或滑鼠／`Enter` 或 `Space` |
| 角色保存庫 | `M` 建庫、`N` 新局、`1`–`8` 選槽、`S` 儲存、`L` 載入 |

各畫面的可用動作見 `F1` 與[完整繁中操作說明](docs/manual/remake-controls-zh-Hant.md)。建立角色時字母鍵用來輸入名字，離開用 `F10`。來源主線只支援 schema45 新存檔，預設檔名包含 v45。已發布 v.1.2.0-20261016 使用 schema44；舊檔保留給舊版讀取，不轉換。

存檔、角色庫與設定使用系統設定目錄：Linux `~/.config/sundog-remake/`、Windows `%APPDATA%\sundog-remake\`、macOS `~/Library/Application Support/sundog-remake/`。

## 驗證與授權

正常玩家抽驗涵蓋新局、船艙入口、導航、地面、城市服務、室內、航行睡眠、結算、失敗終局與保存重開。四語與兩種視窗尺寸另做繪圖檢查。來源主線已整合模式1至7及未開隱形裝置的模式0航向倒數、射擊準備與跨次狀態，原生連續父鏈的受控對拍通過。模式0開啟隱形裝置時仍沿既有近似排程，完整戰鬥仍未驗收，見[戰鬥實作](internal/game/enemy_course.go)。完整原版 GUI、自然亂數同序、全部交戰分支、Windows／macOS 實機及人耳同步仍有證據限制；不宣稱整款遊戲逐像素或逐幀一致。

文件入口見[文件索引](docs/index.md)。部分研究工具仍引用原私人專案的證據路徑，需要自備合法研究輸入；那些輸入未包含在此 repo。一般建置及單元測試可使用此公開來源。

自行撰寫的程式、工具、翻譯與新美術依 [RRSAL-1.0](LICENSE) 提供；外部字型各依其授權，見[字型紀錄](assets/fonts/ASSET-MANIFEST.md)。

《SunDog: Frozen Legacy》及原版素材的權利屬原權利人。本專案為獨立研究重製，與原廠無隸屬關係。原版素材不因本專案授權而取得再散布權。
