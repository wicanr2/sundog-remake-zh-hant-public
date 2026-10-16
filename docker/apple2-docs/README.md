# Apple II／手冊文件工具鏈

這個 image 只處理文件研究：PDF 轉頁面、OCR 與文字抽取。原始 PDF 以唯讀掛載，輸出
使用目前使用者 UID/GID 寫入明確 output directory。

## 建置

```sh
docker build --network=default -t sundog-apple2-docs:20260811 docker/apple2-docs
```

固定套件版本：`poppler-utils 22.12.0-2+deb12u3`、`tesseract-ocr 5.3.0-2`、
`tesseract-ocr-eng 1:4.1.0-2`。

## 使用原則

- `Sundog-Frozen-Legacy_Manual_Atari-ST_EN.pdf` 只讀掛載到 `/input/manual.pdf`。
- OCR 中間頁面放在容器 `/tmp`；最後的 Markdown／校稿輸出才掛載到 repository 的明確
  `docs/manual/` 目錄。
- OCR 結果是候選文字，必須逐頁與原始掃描頁面對照；不可把 OCR 全對當成翻譯或版面完成。
- image 只作工具鏈，不包含原版 PDF、磁碟或遊戲資產。
