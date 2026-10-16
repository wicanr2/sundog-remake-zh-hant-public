# 遊戲語系

目前提供繁體中文 `zh-Hant`、簡體中文 `zh-Hans`、日文 `ja`、韓文 `ko` 與英文 `en`。在遊戲內開啟 F2，以方向鍵切換語言；設定立即生效，重開仍保留。語言只進設定檔，不寫入遊戲狀態或 schema44 存檔。

既有四語沿原 JSON catalog；新增韓文的唯一人工編輯來源為 [ko.tsv](ko.tsv)，欄位為 `family`、穩定 `key` 與 JSON 跳脫的 `text_json`。目前來源包含 826 個介面、46 個 Apple II 詞條與 455 個 Atari ST 對話片段。地名保留可回查的原文拼法，對話的 `{hh}` 引用保持來源結構。

在 Docker 內執行 [build_korean_language_pack.py](../../tools/build_korean_language_pack.py) 產生 `sundog-ui-ko.json`、`apple2-text-ko.json` 與 `atari-st-dialog-ko.json`。這三檔是既有 runtime 格式的產物，不直接改譯文；`--check` 檢查是否與 TSV 同步。

驗證入口為 [validate_language_packs.py](../../tools/validate_language_packs.py) 與 [validate_cjk_font_coverage.py](../../tools/validate_cjk_font_coverage.py)。前者檢查五語 key、格式參數、來源及對話引用，後者以實際字型檢查中文、日文與韓文字形。Noto Sans CJK TC 已涵蓋 11,172 個現代韓文音節，不另引入未授權字型。

翻譯經 AI 製作及結構檢查，尚未由各語言母語編輯完整校閱。文字安全區與正常玩家抽驗僅證明所列畫面與路徑，不代表全部劇情已由母語玩家通關。
