#!/usr/bin/env python3
"""把 Atari ST SunDog 的對話 frame 展開成看得懂的句子。

`docs/atari-st/dialog-frames.json` 解出了 frame 的版面，`docs/atari-st/dialog-text.json`
解出了字串與 64 個 stock phrase，但兩者之間一直缺一段：frame 的 `message` 欄位不是字串
索引，句子也不是字串本身——原版跑的是一台 token 展開機。這支工具補上那一段。

模型全部讀自 `XDOINTER` 的 p-code，沒有一項來自次級文件：

- **`message` 是位元組游標。** `XDOINTER:0x10 put_message(i)` 把區塊游標設成 `i * 2`
  （`0x07de`），再讀一個長度位元組，然後逐位元組展開。所以 `值 × 2` 就是 512-byte 區塊裡
  的位移，指向一個「長度 + 內容」的字串。
- **`message` 小於 30 時它是選擇子，不是指標。** `XDOINTER:0x17 pick_one` **只有值是 22 或 23 時**才減 2
  （`0x0836` 的 `tjp` 在其餘情況跳過減法），值 ≤ 10 時以 `random() mod 值` 從後面跟著的候選位元組裡挑一個
  （`0x0897`），挑中的那個才是真正的指標。`dialog-frames.json` 已經把候選記成 `choices`。
- **內容的每個位元組**：≥ `0x80` 表示展開後補一個空格（先減 `0x80`）。低 7 位元交給
  `XDOINTER:0x11 replace`：`≥ 0x41` 是字面字元；`0x20`–`0x2f` 是一張分派表（下方
  `SPECIALS`）；其餘（`0x00`–`0x1f`、`0x30`–`0x40`）是 stock phrase 引用。
- **變體的挑法有兩種。** `replace` 對一般引用傳 `y = -1`（`0x075c`），`get_token` 因此
  跳過 `get_offset`，複製到第一個 `0x60` 分隔符為止——**取第 0 個變體，不擲骰**。
  只有 `0x29`、`0x2a`、`0x2b` 傳 `y = 0`（`0x0794`），`get_offset` 才會做
  `random() mod 變體數`（`0x0535`）。次級文件說「變體預設隨機」，對這份 binary 不成立。
- **要不要遞迴展開**由 `append` 的第三個參數決定：1 會對複製出來的每個位元組再跑一次
  `replace`（`0x06e3`），0 只是原樣接上（`0x06f4`）。

輸出保留兩種形式：`text` 是渲染後的句子，`raw` 是原始 token 串，兩者都留著才能中文化。
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

# XDOINTER:0x11 replace 的 0x20–0x2f 分派表（跳表在 0x07ca）。
# mode: literal 直接輸出、none 什麼都不做、fixed 固定字串、
#       dynamic 執行時才知道、token0 取第 0 個變體、random 隨機變體、
#       pick 由某個執行時值選變體。
SPECIALS: dict[int, dict[str, object]] = {
    0x20: {"mode": "literal", "text": " "},
    0x21: {"mode": "none"},
    0x22: {"mode": "fixed", "text": "O.K."},
    0x23: {"mode": "dynamic", "text": "«地名»", "note": "XDOINTER:0x0f get_place：各半機率抽星系名或行星名"},
    0x24: {"mode": "dynamic", "text": "«金額»", "note": "XDOINTER_01_L4c，由 set_up_price 寫入"},
    0x25: {"mode": "dynamic", "text": "«物品»", "note": "XDOINTER_01_L54，由 set_up_price 從物品表取出"},
    0x26: {"mode": "pick", "by": "MAINLIB_G10b[11] == 1"},
    0x27: {"mode": "literal", "text": "'"},
    0x28: {"mode": "pick", "by": "MAINLIB_G10b[11] == 1"},
    0x29: {"mode": "random"},
    0x2A: {"mode": "random"},
    0x2B: {"mode": "random"},
    0x2C: {"mode": "literal", "text": ","},
    0x2D: {"mode": "pick", "by": "XDOINTER_01_L1b", "recurse": True},
    0x2E: {"mode": "none"},
    0x2F: {"mode": "pick", "by": "XDOINTER_01_Lc"},
}

TOKEN_LIMIT = 0x40  # get_token 對 x >= 0x40 直接回空字串（0x0576）
RECURSION_LIMIT = 8


class Renderer:
    """一個區塊的展開器。

    token 表不是只有 64 個全域 stock phrase：`get_adr` 走的 `XDOINTER_01_L164` 在載入區塊時
    會被該區塊自己的 local token 覆蓋高段（`0x34` 起）。同一個編號在不同區塊指的是不同的
    詞——酒館的 `0x34` 是「please」，旅館的是「looking for|need」——所以展開器必須逐區塊建。
    """

    def __init__(self, text_document: dict, block: dict | None = None) -> None:
        self.phrases: dict[int, list[str]] = {}
        for record in text_document["stockPhrases"]["records"]:
            index = int(str(record["index"]), 16)
            self.phrases[index] = [self._raw(a) for a in record["alternatives"]]
        if block is not None:
            for entry in block["strings"]:
                if entry.get("role") != "block-phrase":
                    continue
                reference = entry.get("referenceIndex")
                if reference is None:
                    continue
                self.phrases[int(str(reference), 16)] = [
                    self._raw(a) for a in entry.get("alternatives", [])
                ]

    @staticmethod
    def _raw(alternative: list[dict]) -> str:
        out = []
        for part in alternative:
            if part.get("kind") == "text":
                out.append(str(part["text"]))
            else:
                out.append(f"\x00{int(str(part['index']), 16):02x}\x00")
        # get_token 把 0x7e 換成單引號（0x05e5）；line 那條路沒有這一步。
        return "".join(out).replace("~", "'")

    def _expand_raw(self, raw: str, depth: int) -> str:
        """把 _raw 產生的內嵌引用標記再跑一次 replace。"""
        out: list[str] = []
        cursor = 0
        while cursor < len(raw):
            if raw[cursor] == "\x00":
                index = int(raw[cursor + 1 : cursor + 3], 16)
                out.append(self.reference(index, depth + 1))
                cursor += 4
            else:
                out.append(raw[cursor])
                cursor += 1
        return "".join(out)

    def variant(self, index: int, which: int, depth: int, recurse: bool = True) -> str:
        alternatives = self.phrases.get(index)
        if not alternatives:
            return ""
        raw = alternatives[which % len(alternatives)]
        return self._expand_raw(raw, depth) if recurse else raw.replace("\x00", "")

    def reference(self, index: int, depth: int = 0) -> str:
        if depth > RECURSION_LIMIT:
            return "«遞迴過深»"
        if index >= TOKEN_LIMIT and index not in SPECIALS:
            return ""
        special = SPECIALS.get(index)
        if special is not None:
            mode = special["mode"]
            if mode == "none":
                return ""
            if mode in ("literal", "fixed", "dynamic"):
                return str(special["text"])
            alternatives = self.phrases.get(index) or []
            if mode == "random":
                rendered = [self._expand_raw(a, depth) for a in alternatives]
                rendered = [r for r in rendered if r] or [""]
                return rendered[0] if len(rendered) == 1 else "{" + "|".join(rendered) + "}"
            # pick：由執行時的值選變體，兩個都列出來
            rendered = [self._expand_raw(a, depth) for a in alternatives]
            return "[" + "|".join(rendered) + "]" if len(rendered) > 1 else "".join(rendered)
        # 一般引用：replace 傳 y = -1，取第 0 個變體
        return self.variant(index, 0, depth)

    def render(self, tokens: list[dict]) -> str:
        out: list[str] = []
        for part in tokens:
            if part.get("kind") == "text":
                out.append(str(part["text"]))
            else:
                out.append(self.reference(int(str(part["index"]), 16)))
        text = "".join(out)
        while "  " in text:
            text = text.replace("  ", " ")
        text = text.strip()
        return text[:1].upper() + text[1:] if text else text


def describe_condition(code: int) -> dict[str, object]:
    """把 `pick_one` 帶的條件碼翻成 `XDOINTER:0x06 passed` 的語意。

    `passed` 先把參數拆成「選哪一項檢定」加參數（`0x0245`–`0x0298`）：
    `>= 200` → 檢定 20、參數 `碼 − 200`；`>= 100` → 檢定 19、參數 `碼 − 100`；
    `>= 90` → 檢定 15；`>= 20` → 另一套十進位拆法。檢定 20 是「目前談的物品編號等於參數」
    （`0x0412`），檢定 19 是 `made_roll(參數)`，也就是 `random() mod 100 < 參數`（`0x0206`）。
    """
    if code >= 200:
        return {"code": code, "check": "item", "detail": f"目前談的物品編號等於 {code - 200}"}
    if code >= 100:
        return {"code": code, "check": "roll", "detail": f"random() mod 100 < {code - 100}"}
    return {"code": code, "check": "other", "detail": "見 passed 的分派表（0x0419）"}


def build(frames_path: Path, text_path: Path) -> dict[str, object]:
    frames_document = json.loads(frames_path.read_text(encoding="utf-8"))
    text_document = json.loads(text_path.read_text(encoding="utf-8"))
    strings_by_block: dict[int, dict[int, dict]] = {}
    renderers: dict[int, Renderer] = {}
    for block in text_document["dialogBlocks"]:
        number = int(str(block["blockNumber"]), 16)
        base = int(str(block["rawOffset"]), 16)
        strings_by_block[number] = {
            int(str(entry["rawOffset"]), 16) - base: entry for entry in block["strings"]
        }
        renderers[number] = Renderer(text_document, block)
    fallback = Renderer(text_document)

    rendered_frames = []
    misses = 0
    total_pointers = 0
    for block in frames_document["blocks"]:
        for frame in block["frames"]:
            identifier = int(frame["id"])
            number = identifier & 0xF0
            lookup = strings_by_block.get(number, {})
            renderer = renderers.get(number, fallback)
            # message >= 30 是直接指標；否則 pick_one 從 choices 裡挑一個。
            # chain frame 挑的是**下一個 frame 的編號**，不是文字指標：它沒有 message，
            # set_frame 讀到的那個值一路交給 new_frame。所以這裡只記轉向，不當成漏掉。
            if frame.get("kind") == "chain":
                targets = frame.get("choices") or [int(frame["target"])]
                rendered_frames.append(
                    {
                        "id": f"0x{identifier:02x}",
                        "block": f"0x{number:02x}",
                        "index": frame["index"],
                        "kind": "chain",
                        "selection": "random" if len(targets) > 1 else "fixed",
                        "chainsTo": [f"0x{int(t):02x}" for t in targets],
                    }
                )
                continue
            choices = frame.get("choices") or []
            kinds = frame.get("choiceKinds") or ["pointer"] * len(choices)
            if not choices and frame.get("message") is not None:
                choices, kinds = [int(frame["message"])], ["pointer"]
            lines = []
            pending_condition = None
            for value, kind in zip(choices, kinds):
                if kind == "condition":
                    # 這個碼交給 XDOINTER:0x06 passed 判；通過就用**前一句**。
                    pending_condition = int(value)
                    if lines:
                        lines[-1]["condition"] = describe_condition(pending_condition)
                    continue
                total_pointers += 1
                entry = lookup.get(int(value) * 2)
                if entry is None:
                    misses += 1
                    continue
                lines.append(
                    {
                        "pointer": value,
                        "byteOffset": int(value) * 2,
                        "rawOffset": entry["rawOffset"],
                        "raw": entry["preview"],
                        "text": renderer.render(entry["tokens"]),
                    }
                )
            if not lines:
                continue
            rendered_frames.append(
                {
                    "id": f"0x{identifier:02x}",
                    "block": f"0x{number:02x}",
                    "index": frame["index"],
                    "kind": frame.get("kind"),
                    "selection": "random" if len(lines) > 1 else "fixed",
                    "lines": lines,
                }
            )

    return {
        "schema": 1,
        "tool": "tools/atari_st_dialog_render.py",
        "inputs": {
            "frames": str(frames_path),
            "text": str(text_path),
            "framesTool": frames_document.get("tool"),
            "textTool": text_document.get("tool"),
        },
        "evidenceLevel": "proven",
        "model": (
            "message × 2 是區塊內的位元組位移（XDOINTER:0x10 put_message，0x07de）；"
            "message < 30 時先經 pick_one 隨機挑一個候選（0x0897）。"
            "一般引用取第 0 個變體（replace 傳 y = -1，0x075c），"
            "只有 0x29／0x2a／0x2b 隨機挑變體（0x0794 → get_offset 0x0535）。"
        ),
        "notation": {
            "{a|b}": "原版隨機挑一個變體",
            "[a|b]": "由執行時的值選一個（欄位見 SPECIALS）",
            "«…»": "執行時才產生的內容（地名、金額、物品名）",
        },
        "pointers": {"total": total_pointers, "unresolved": misses},
        "choiceModel": (
            "候選清單有兩種形狀。message > 10 那一支是「指標、條件」交替，條件碼交給 "
            "XDOINTER:0x06 passed 判，通過就用前一句，最後一個沒有條件、是預設；"
            "其餘是清一色的指標，由 random() 挑一個。"
        ),
        "frames": rendered_frames,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--frames", type=Path, default=Path("docs/atari-st/dialog-frames.json"))
    parser.add_argument("--text", type=Path, default=Path("docs/atari-st/dialog-text.json"))
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    document = build(args.frames, args.text)
    payload = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")


if __name__ == "__main__":
    main()
