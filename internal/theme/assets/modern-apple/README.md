# Modern Apple 正式高解析 atlas v1

這個目錄保存可再散布的 remake-owned 正式輸出，不保存任何 Apple II／Atari ST／原版遊戲圖像。

- `modern-apple-atlas.png`：512×256 PNG，8 個 128×128 semantic cell。
- `modern-apple-atlas-manifest.json`：atlas 雜湊、來源 artifact 雜湊、alpha 統計、語意格與原版證據邊界。
- `modern-apple-atlas-contact.png`：由正式 atlas 產生的接觸表，只供人工檢查。

輸入來源圖不是 runtime 依賴，也不在 repository。需要重建時，必須在 Docker 中使用
`tools/build_modern_apple_atlas.py` 與 imagegen 色鍵 helper，並核對 manifest／atlas 雜湊。
不得手動改 PNG 後保留舊 manifest，也不得把低解析 v2、Apple II 或 Atari ST 圖直接放大替換此 atlas。

這一批覆蓋目前 map 的地形與 object cell。第二批鋪面／太空港圖塊集已於 2026-08-30
退役並移除；相關沿革見 `PLAN.md` §0.96。目前這份 atlas 以 Remake Modern 圖塊集使用，
manifest 保留 `modern-apple` 作已稽核的批次身份。歷史 runtime capture 見
[`docs/art/modern-apple-highres-runtime-evidence.json`](../../../../docs/art/modern-apple-highres-runtime-evidence.json)。
