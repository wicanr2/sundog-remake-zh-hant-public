# 故事影片來源與重建

故事開場見 [five-language-story.json](five-language-story.json)，依精訊資訊《陽犬號2.0版》手冊印刷頁4–9重新撰寫。原始RAR與掃描未隨公開來源提供。

- [故事卡生成器](../../tools/build_story_promo_cards.py)：在Docker內指定 `--story`、`--font`、新的 `--output`，製作獨立文字卡並檢查文字安全區。
- [五語正常操作與錄影](../../tools/verify_locale_player.py)：指定正式 `--binary`、自備 `--disk`、正常 `--settings` 與新的 `--output`。可從正式標題開始，或用已有正常船艙 `--save`。
- [HD 合成器](../../tools/build_hd_promo.py)：使用 `--delivery dist-all/<版本>` 與 `--scenes` 合成。原版音效須由玩家自己的磁碟匯出，含原版音效的影片只留本機。

遊戲錄影、故事文字卡與靜態玩家截圖各自標示。影片不證明全款原版 parity，也不宣稱後製音效為現場同步收音。
