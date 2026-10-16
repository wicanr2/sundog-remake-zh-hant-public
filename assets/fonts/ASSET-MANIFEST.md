# 字型資產 manifest

| 檔案 | 來源 | SHA-256 | 大小 | 授權 |
|---|---|---|---:|---|
| `NotoSansCJKtc-Regular.otf` | [`notofonts/noto-cjk` TraditionalChinese OTF](https://github.com/notofonts/noto-cjk/blob/main/Sans/OTF/TraditionalChinese/NotoSansCJKtc-Regular.otf) | `dce08bd4fd91aa8aa76ed8fea4b694c2dfb8550f67871e326843212ddbeb88b4` | 16,435,884 bytes | SIL Open Font License 1.1 |
| `LICENSE_OFL.txt` | [`notofonts/noto-cjk` Sans license](https://github.com/notofonts/noto-cjk/blob/main/Sans/LICENSE) | `6a73f9541c2de74158c0e7cf6b0a58ef774f5a780bf191f2d7ec9cc53efe2bf2` | 4,301 bytes | SIL Open Font License 1.1 |
| `UKaiTW-SunDogTitleSubset.ttf` | Debian `fonts-arphic-ukai` 的 `ukai.ttc` 第 3 面（AR PL UKai TW）子集 | `13a1894f2dde2bd950274da775d0becf28adb0a3be6c3f92990cb20d957c6aa8` | 26,492 bytes | ARPHIC PUBLIC LICENSE |
| `LICENSE_ARPHIC.txt` | `fonts-arphic-ukai` 的 `license/english/ARPHICPL.TXT` | `3a5e90c0957524a89e48203febcd4492ca4393678abaa7e5b4d70f3ff32b386d` | 6,901 bytes | ARPHIC PUBLIC LICENSE |
| `LICENSE_ARPHIC_zh-Hant.txt` | 同上的 `license/zh_TW.UTF-8/ARPHICPL.TXT` | `7f709381dd578588d8c5b32ee2c9fa2a4995b991ed120605075fe6d2c7951e2a` | 5,812 bytes | ARPHIC PUBLIC LICENSE |

下載日期：2026-08-11。字型檔是 remake 的外部開源資產，不是 SunDog 原版字型，也不作為
Apple II 圖形／字型 parity 證據。

`UKaiTW-SunDogTitleSubset.ttf` 已退出 runtime 與正式封包；本段只保留歷史來源與授權回查。
它是**修改過的版本**，ARPHIC PUBLIC LICENSE 要求顯著標示：
它只保留標題「陽犬號中文版」六個字的字形，其餘全部移除；字形外框未經改動。原字型
(C) 1999 Arphic Technology Co., Ltd.。依該授權，本子集與其衍生物同樣以 ARPHIC PUBLIC
LICENSE 散布，授權全文見同目錄的 `LICENSE_ARPHIC.txt`（中文版 `LICENSE_ARPHIC_zh-Hant.txt`）。
子集是這樣產生的：

```sh
pyftsubset ukai.ttc --font-number=2 --text=陽犬號中文版 --output-file=UKaiTW-SunDogTitleSubset.ttf
```

只留六個字是因為整份 `ukai.ttc` 有 17 MB，而當時的重製版標題只需要那六個字形。
