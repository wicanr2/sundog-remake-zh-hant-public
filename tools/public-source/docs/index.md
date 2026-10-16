# 文件索引

- [專案介紹、故事與 HD 畫廊](../README.md)
- [目前狀態與驗證限制](../CONTEXT.md)
- [建置及測試](build.md)
- [五語資料與韓文 TSV 來源](i18n/README.md)
- [精訊故事影片與重建工具](promo/README.md)
- [完整繁中操作說明](manual/remake-controls-zh-Hant.md)
- [公開檔案與截圖雜湊](../PUBLIC-SOURCE-MANIFEST.json)
- [字型來源與授權](../assets/fonts/ASSET-MANIFEST.md)
- [工作紀錄](../WORKLOG.md)
- [工作規約](../AGENTS.md)

`docs/i18n/` 保存執行期翻譯資料；`docs/atari-st/` 僅保留普通單元測試讀取的獨立解碼事實及受控原版答案。原始磁碟、原版對拍圖片及完整研究歷程未隨公開 repo 提供。

`cmd/` 為程式入口，`internal/` 為規則、解析器及呈現，`tools/` 保存建置與研究工具，`docker/` 保存工具鏈。研究工具可能需要私人證據或自備磁碟，請先檢查參數與說明。

公開來源匯出腳本 [prepare_public_repository.py](../tools/prepare_public_repository.py) 供原私人工作樹建立新快照，使用 [public-source 範本](../tools/public-source/README.md)；公開 clone 已是匯出結果，不具備再次匯出所需的私人截圖路徑。
