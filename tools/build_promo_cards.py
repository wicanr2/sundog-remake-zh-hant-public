#!/usr/bin/env python3
"""產生推廣影片用的繁中說明卡與並列對照卡。

卡片尺寸與遊戲畫面一致（960×600），配色沿用 remake 的 Modern Apple 介面配色，
所以卡片與實機畫面接在一起不會跳。文字全部寫在這支工具裡，改字不必改影片流程。

卡片只陳述已經有證據的事：每一條「原版」都指得出解在哪一份文件。
"""

from __future__ import annotations

import argparse
import textwrap
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


WIDTH, HEIGHT = 960, 600
# 註腳從這一行起畫，所以正文最多只能排到這裡。
FOOTNOTE_TOP = HEIGHT - 92
BACKGROUND = (18, 21, 24)
PANEL = (26, 28, 30)
CREAM = (216, 192, 157)
ACCENT = (202, 145, 49)
TEAL = (54, 145, 190)
MUTED = (150, 140, 122)


def load_font(font_path: Path, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(str(font_path), size)


def text_width(draw: ImageDraw.ImageDraw, text: str, font: ImageFont.FreeTypeFont) -> int:
    return int(draw.textlength(text, font=font))


# 連續的西文字母、數字與這些符號視為一個不可拆的詞。中文可以逐字斷，
# 但 `logical block 0x22` 斷在字中間會變成 "bloc / k 0x22"。
LATIN_RUN = set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-/:×")


def tokenize(text: str) -> list[str]:
    """把字串切成斷行單位：西文詞整塊，其餘逐字。"""
    tokens: list[str] = []
    run = ""
    for char in text:
        if char in LATIN_RUN:
            run += char
            continue
        if run:
            tokens.append(run)
            run = ""
        tokens.append(char)
    if run:
        tokens.append(run)
    return tokens


# 行首禁則：這些字不能出現在一行的開頭。中文排版的基本規矩，逐字斷行很容易踩到——
# 一個逗號被推到下一行的開頭，看起來就像排版壞掉。
NO_LINE_START = "，。、；：？！）」』】》〉…－"


def wrap_cjk(draw: ImageDraw.ImageDraw, text: str, font: ImageFont.FreeTypeFont, limit: int) -> list[str]:
    """逐字量寬度斷行。中文沒有空白可斷，textwrap 幫不上忙。"""
    lines: list[str] = []
    current = ""
    for token in tokenize(text):
        if token == "\n":
            lines.append(current)
            current = ""
            continue
        if text_width(draw, current + token, font) > limit and current:
            # 這一個字放不下，但它又不能當行首——那就讓上一行多擠一個字，
            # 把它連同前一個字一起帶下來。
            if token in NO_LINE_START:
                current += token
                continue
            lines.append(current)
            current = token.lstrip(" ")
        else:
            current += token
    if current:
        lines.append(current)
    return lines


def title_card(fonts: dict[str, ImageFont.FreeTypeFont], heading: str, subtitle: str, footnote: str) -> Image.Image:
    card = Image.new("RGB", (WIDTH, HEIGHT), BACKGROUND)
    draw = ImageDraw.Draw(card)
    draw.rectangle((0, 0, WIDTH, 6), fill=ACCENT)
    draw.rectangle((0, HEIGHT - 6, WIDTH, HEIGHT), fill=ACCENT)
    heading_font = fonts["heading"]
    y = 200
    width = text_width(draw, heading, heading_font)
    draw.text(((WIDTH - width) // 2, y), heading, font=heading_font, fill=CREAM)
    y += 96
    width = text_width(draw, subtitle, fonts["subtitle"])
    draw.text(((WIDTH - width) // 2, y), subtitle, font=fonts["subtitle"], fill=TEAL)
    if footnote:
        y = HEIGHT - 110
        width = text_width(draw, footnote, fonts["small"])
        draw.text(((WIDTH - width) // 2, y), footnote, font=fonts["small"], fill=MUTED)
    return card


def section_card(fonts: dict[str, ImageFont.FreeTypeFont], heading: str, rows: list[tuple[str, str]], footnote: str) -> Image.Image:
    """左欄寫原版解出了什麼，右欄寫 remake 拿它做了什麼。"""
    card = Image.new("RGB", (WIDTH, HEIGHT), BACKGROUND)
    draw = ImageDraw.Draw(card)
    draw.rectangle((0, 0, WIDTH, 6), fill=ACCENT)
    draw.text((56, 44), heading, font=fonts["subtitle"], fill=ACCENT)

    left_x, right_x = 56, 500
    column = 404
    y = 128
    draw.text((left_x, y), "原版磁碟解出來的", font=fonts["label"], fill=TEAL)
    draw.text((right_x, y), "重製版做了什麼", font=fonts["label"], fill=TEAL)
    y += 44
    draw.line((left_x, y - 12, WIDTH - 56, y - 12), fill=PANEL, width=2)

    for original, remake in rows:
        left_lines = wrap_cjk(draw, original, fonts["body"], column - 24)
        right_lines = wrap_cjk(draw, remake, fonts["body"], column - 24)
        height = max(len(left_lines), len(right_lines)) * 34
        draw.text((left_x, y), "\n".join(left_lines), font=fonts["body"], fill=CREAM, spacing=8)
        draw.text((right_x, y), "\n".join(right_lines), font=fonts["body"], fill=CREAM, spacing=8)
        y += height + 26

    # 卡片高度是固定的，列數與換行卻不是。多加一列或多一次換行就會壓到註腳，而 PIL
    # 不會抱怨——它照樣畫，畫出去的部分就沒了。所以在這裡擋下來，讓它變成建置失敗
    # 而不是影片裡一段看不懂的重疊文字。
    if y > FOOTNOTE_TOP:
        raise ValueError(
            f"卡片「{heading}」的內容排到 y={y}，會壓到 y={FOOTNOTE_TOP} 的註腳；"
            "請減少列數或縮短文字"
        )
    if footnote:
        for index, line in enumerate(wrap_cjk(draw, footnote, fonts["small"], WIDTH - 112)):
            draw.text((56, FOOTNOTE_TOP + index * 26), line, font=fonts["small"], fill=MUTED)
    return card


def side_by_side(
    fonts: dict[str, ImageFont.FreeTypeFont],
    left_image: Path,
    right_image: Path,
    left_label: str,
    right_label: str,
    caption: str,
) -> Image.Image:
    card = Image.new("RGB", (WIDTH, HEIGHT), BACKGROUND)
    draw = ImageDraw.Draw(card)
    draw.rectangle((0, 0, WIDTH, 6), fill=ACCENT)

    panel_width, panel_height = 440, 300
    top = 120
    for index, (path, label) in enumerate(((left_image, left_label), (right_image, right_label))):
        x = 30 + index * (panel_width + 20)
        with Image.open(path) as source:
            shot = source.convert("RGB")
        shot.thumbnail((panel_width, panel_height), Image.Resampling.LANCZOS)
        offset = (x + (panel_width - shot.width) // 2, top + (panel_height - shot.height) // 2)
        draw.rectangle((x - 2, top - 2, x + panel_width + 2, top + panel_height + 2), outline=PANEL, width=2)
        card.paste(shot, offset)
        width = text_width(draw, label, fonts["label"])
        draw.text((x + (panel_width - width) // 2, top - 46), label, font=fonts["label"], fill=TEAL)

    y = top + panel_height + 40
    for line in wrap_cjk(draw, caption, fonts["body"], WIDTH - 112):
        draw.text((56, y), line, font=fonts["body"], fill=CREAM)
        y += 36
    return card


def build(font_path: Path, shots: Path, output: Path) -> list[Path]:
    fonts = {
        "heading": load_font(font_path, 64),
        "subtitle": load_font(font_path, 34),
        "label": load_font(font_path, 24),
        "body": load_font(font_path, 24),
        "small": load_font(font_path, 19),
    }
    output.mkdir(parents=True, exist_ok=True)
    cards: list[tuple[str, Image.Image]] = []

    cards.append((
        "01-title",
        title_card(
            fonts,
            "陽犬號 SunDog",
            "Frozen Legacy — 繁體中文重製版",
            "原版 1985 年 FTL Games／Software Heaven。本片為研究性重製版展示。",
        ),
    ))

    cards.append((
        "02-what-is-this",
        section_card(
            fonts,
            "這是什麼",
            [
                ("1985 年的 Atari ST 版磁碟，遊戲邏輯寫在 UCSD p-code 裡", "把磁碟上的資料表解出來，用 Go 重寫成可玩的重製版"),
                ("遊戲內文字全部是英文", "介面、對話、物品、地名全繁體中文；專有名詞標成 中文(英文)"),
                ("執行需要 Atari ST 或模擬器", "單一執行檔，鍵盤與滑鼠操作"),
            ],
            "規則是從原版的 p-code 讀出來的，資料表直接取自磁碟；原版的程式碼、圖像與音訊都沒有進重製版。",
        ),
    ))

    cards.append((
        "03-world-data",
        section_card(
            fonts,
            "世界與城市：資料直接來自原版磁碟",
            [
                ("60 座城市的地表地圖，logical block 0x22 起，一座 128 bytes", "60 座城市各畫自己的 20×12 地圖，不共用原型地圖"),
                ("每座城市的建築槽位：接駁站、倉庫、交易所", "走到哪一格開哪一個設施，依建築本身的身分"),
                ("星系、行星、城市名與極座標表", "導航目標與航程長度由那張表算出來"),
            ],
            "版面證據見 docs/atari-st/city-maps.md；每一條規則都以磁碟位元組簽章釘住（287 條）。",
        ),
    ))

    cards.append((
        "04-tiles-palette",
        section_card(
            fonts,
            "圖塊與顏色：解出的是結構，不是素材",
            [
                ("地形 4-bit 碼先轉成畫面碼，再分三段挑三條圖塊條", "重製版照同一個結構：兩個專用塊，十三種地形各有兩種樣子"),
                ("開機寫定的 16 色調色盤；畫地表只換第 5 格", "顏色結構已記錄；重製版用自己的高解析美術"),
                ("圖塊像素本身留在原版磁碟裡", "四套可切換圖塊集，全部是重製版自己畫的；另可即時解出原版圖塊"),
            ],
            "像素不入庫：重製版只帶解碼器，指向自己的原版磁碟才畫得出原版圖塊。",
        ),
    ))

    cards.append((
        "05-systems",
        section_card(
            fonts,
            "規則：從磁碟讀出來，不是猜的",
            [
                ("六套船艦系統、分流器、傷害與命中公式", "工程艙的維修與戰鬥結果照原版公式算"),
                ("價格公式與八間店的存貨清單", "市場價格與可買清單由原版資料決定"),
                ("99 個對話 frame 的版面與文字串流", "對話系統照原版版面走，文字換成繁體中文"),
            ],
            "每一條都附一段磁碟位元組簽章，且要求在整份 SYSTEM.STARTUP 內唯一——記錯位址就對不上。",
        ),
    ))

    cards.append((
        "06-presentation",
        section_card(
            fonts,
            "呈現拆成兩軸",
            [
                ("一種介面、一種圖塊，沒得選", "介面配色五種、圖塊集四種，是兩個各自獨立的軸"),
                ("畫面顏色由開機寫定的 16 色決定", "F8 只換介面配色，F9 只換圖塊集，兩者互不牽動"),
                ("圖塊由城市地圖標頭選三條", "20 種組合在啟動時全部驗過一次，玩家選得到的都畫得出來"),
            ],
            "切換不改變地圖語意、碰撞、狀態或存檔；兩軸都寫進存檔。",
        ),
    ))

    cards.append((
        "07-restored",
        section_card(
            fonts,
            "補完：原版有、重製版原本沒有的",
            [
                ("角色一次拿一件在手上", "手上一格、身上四格，H 在兩者之間搬"),
                ("導航是三張圖", "星圖、星系圖、行星圖都畫出來，座標解自磁碟"),
                ("貨艙一位可換成輔助燃料槽", "主燃料見底時滿槽自動注入"),
                ("街上會有人來搭話", "腳下的地形碼決定是哪一種"),
            ],
            "伸手要錢的、乞討的、來談生意的——要錢的那個先把話說完才動手，走出對話才開打，"
            "全程不扣現金，那是原版的流程。行星公轉、違停罰金與領航台電車也都照原版的算式。",
        ),
    ))

    cards.append((
        "08-original-tiles",
        section_card(
            fonts,
            "原版圖塊：執行時從你自己的磁碟解",
            [
                ("圖塊藏在 4-bit 壓縮串流裡，88 筆影像目錄", "重製版帶的是解碼器，指向你的 .st 就即時解出來畫"),
                ("開機寫定的 16 色調色盤", "同一組顏色；解碼器與調色盤由標題畫面同時釘住"),
                ("素材與程式綁在同一張磁碟上", "沒有磁碟就用重製版自己的美術，一樣完整可玩"),
            ],
            "解碼器驗證的方式：影像 0 解出來要與 Hatari 跑原版擷取到的標題畫面一致。",
        ),
    ))

    cards.append((
        "09-sound",
        section_card(
            fonts,
            "音效：磁碟上沒有音訊，只有指令流",
            [
                ("聲音是 XBIOS 32（Dosound）的暫存器指令流", "28 段加三段 warp，解自磁碟後跑進 YM2149 的模型"),
                ("由 ST 上那顆 2 MHz 的聲音晶片唱出來", "晶片時脈以 Hatari 實機錄音校正過，音高與包絡都對得上"),
                ("聲音位元組屬於原版磁碟", "離線轉成 OGG 放進你的使用者資料夾，重製版不收聲音"),
            ],
            "現在聽到的是實際轉出來的八段：選單、雷射、爆炸、傳送、加油、通訊、跳躍、警報。",
        ),
    ))

    cards.append((
        "13-platform-looks",
        section_card(
            fonts,
            "同一個遊戲，兩個平台的樣子",
            [
                ("Apple II 版是 1984 年的，280×192 的 HGR 畫面", "Apple II 配色與圖塊集：黑底、白框、綠與金"),
                ("Atari ST 版是 1985 年的，16 色調色盤", "Atari ST 配色與圖塊集：深藍底、藍與磚紅"),
                ("兩個版本的畫面各自寫死在各自的磁碟裡", "同一份存檔、同一個位置，只有呈現換掉"),
            ],
            "接下來兩段走的是同一條路：同樣的城市、同樣的格子、同樣的畫面順序，只有樣式不同。",
        ),
    ))

    cards.append((
        "14-atari-st-interface",
        section_card(
            fonts,
            "Atari ST 介面：照原版的框與視窗",
            [
                ("七塊框、量表與燈號的位置量自磁碟上的影像目錄", "駕駛台、戰術顯示器、工程艙都在原版的格子裡"),
                ("每個視窗的位置與大小讀自 p-code 的 make_zoom", "補給站、交易所、對話都開在原版開的那一格"),
                ("掃描儀圓、系統圖、六帶燈號逐像素對過 Hatari 快照", "畫的是重製版的圖塊集與繁體中文，不是原版像素"),
            ],
            "接下來這一段整段都在 Atari ST 介面：駕駛台、戰術、工程艙，然後落地開補給站。",
        ),
    ))

    original_title = shots / "original-title.png"
    remake_title = shots / "remake-title.png"
    if original_title.is_file() and remake_title.is_file():
        cards.append((
            "10-side-by-side",
            side_by_side(
                fonts,
                original_title,
                remake_title,
                "原版 Atari ST（Hatari）",
                "繁體中文重製版",
                "同一個起點：原版要先由 TOS 開機再執行 SUNDOG.PRG，重製版是單一執行檔直接進標題。",
            ),
        ))

    original_menu = shots / "original-menu.png"
    remake_ground = shots / "remake-ground.png"
    if original_menu.is_file() and remake_ground.is_file():
        cards.append((
            "11-side-by-side-2",
            side_by_side(
                fonts,
                original_menu,
                remake_ground,
                "原版主選單",
                "重製版城市地面",
                "原版標題的 SUNDOG 標誌會做調色盤循環，那段程式在 MAINLIB:0x32；重製版城市地圖的每一格都來自原版磁碟。",
            ),
        ))

    cards.append((
        "12-closing",
        title_card(
            fonts,
            "陽犬號 SunDog",
            "繁體中文重製版",
            "重製版不含任何原版圖像、音訊或程式碼。原版著作權屬原權利人。",
        ),
    ))

    written = []
    for name, card in cards:
        path = output / f"{name}.png"
        card.save(path)
        written.append(path)
    return written


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font", type=Path, default=Path("assets/fonts/NotoSansCJKtc-Regular.otf"))
    parser.add_argument("--shots", type=Path, required=True, help="並列對照用的截圖目錄")
    parser.add_argument("--output", type=Path, required=True, help="卡片輸出目錄")
    args = parser.parse_args()
    written = build(args.font, args.shots, args.output)
    for path in written:
        print(path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
