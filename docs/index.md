# 文件索引

- [專案介紹、故事與 HD 畫廊](../README.md)
- [五語 HD 正常玩家截圖](../README.md#五語-hd-遊玩截圖)
- [目前狀態與驗證限制](../CONTEXT.md)
- [建置及測試](build.md)
- [五語資料與韓文 TSV 來源](i18n/README.md)
- [精訊故事影片與重建工具](promo/README.md)
- [v.1.2.0 五語與三平台驗收](atari-st/release-v1200-20261016.json)
- [完整繁中操作說明](manual/remake-controls-zh-Hant.md)
- [倉庫 Swap 正常玩家驗證工具](../tools/verify_warehouse_swap_player.py)
- [連續航行燃料父層原生答案與驗證範圍](atari-st/fuel-flight-parent-parity-20261016.json)
- [schema44正常購油、兩補油入口與五語畫面驗收](atari-st/fuel-normal-player-parity-20261016.json)
- [正常燃料玩家驗證工具](../tools/verify_fuel_player.py)
- [公開檔案與截圖雜湊](../PUBLIC-SOURCE-MANIFEST.json)
- [字型來源與授權](../assets/fonts/ASSET-MANIFEST.md)
- [工作紀錄](../WORKLOG.md)
- [工作規約](../AGENTS.md)

`docs/i18n/` 保存執行期翻譯資料；`docs/atari-st/` 僅保留普通單元測試讀取的獨立解碼事實及受控原版答案。原始磁碟、原版對拍圖片及完整研究歷程未隨公開 repo 提供。

`cmd/` 為程式入口，`internal/` 為規則、解析器及呈現，`tools/` 保存建置與研究工具，`docker/` 保存工具鏈。研究工具可能需要私人證據或自備磁碟，請先檢查參數與說明。

Swap 驗證使用自備合法磁碟、正常Dramming取貨保存及足以購買兩批貨物的資金；參數見工具 `--help`。`--review-only` 從已有正常倉庫保存重拍五語畫面，不執行購買或交換。這些輸入不隨公開來源提供。

公開來源匯出腳本 [prepare_public_repository.py](../tools/prepare_public_repository.py) 供原私人工作樹建立新快照，使用 [public-source 範本](../tools/public-source/README.md)；公開 clone 已是匯出結果，不具備再次匯出所需的私人截圖路徑。
