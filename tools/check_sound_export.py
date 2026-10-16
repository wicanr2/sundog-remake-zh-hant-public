#!/usr/bin/env python3
"""對轉出的音效做客觀健檢。

這**不是**人耳驗收（WORKLIST D3 要的是使用者的耳朵，建置機沒有音效輸出）。它做的是
另一件事：把「人耳一聽就會發現的問題」換成可重跑的量測。三類會亮紅燈：

* **削波**——連續三個以上樣本頂到滿刻度。方波晶片本來就會踩到滿刻度，所以判準是
  連續而不是單點。
* **全靜音**——峰值為 0。音效表第 0 段是「停掉背景音」，本來就該是靜音，所以它是
  唯一的預期例外，寫在 EXPECTED_SILENT 裡。
* **幾乎聽不到**——峰值不到滿刻度的 1%。

**直流偏移不算問題**，只記錄。YM2149 的方波佔空比不對稱，輸出本來就偏一邊；這些樣本
與 `laanwj/sundog` 的輸出逐 sample 相同（`compare_dosound_oracle.sh`），所以偏移是
晶片模型的特性而不是轉檔的缺陷。喇叭也不會把直流播出來。

全部通過只能說「沒有這三類問題」，不能說「聽起來對」。那一步仍然要人耳。
"""

from __future__ import annotations

import argparse
import json
import pathlib
import struct
import sys
import wave

# 音效表第 0 段是「停掉背景音」，靜音是它的正確內容（docs/atari-st/sound-events.md）。
EXPECTED_SILENT = {"00-off"}

CLIP_RUN = 3
QUIET_PEAK_PCT = 1.0


def read_wav(path: pathlib.Path) -> tuple[list[int], int, float]:
    with wave.open(str(path), "rb") as handle:
        frames = handle.getnframes()
        width = handle.getsampwidth()
        rate = handle.getframerate()
        raw = handle.readframes(frames)
    if width == 1:
        return [b - 128 for b in raw], rate, 127.0
    if width == 2:
        return list(struct.unpack(f"<{len(raw) // 2}h", raw)), rate, 32767.0
    raise ValueError(f"unsupported sample width {width} in {path.name}")


def analyse(path: pathlib.Path) -> dict:
    samples, rate, full = read_wav(path)
    stem = path.stem
    if not samples:
        return {"name": path.name, "stem": stem, "seconds": 0.0, "peakPct": 0.0,
                "rmsPct": 0.0, "dcPct": 0.0, "clippedRuns": 0, "empty": True}
    peak = max(abs(v) for v in samples)
    rms = (sum(v * v for v in samples) / len(samples)) ** 0.5
    dc = sum(samples) / len(samples)
    clipped = 0
    run = 0
    for value in samples:
        run = run + 1 if abs(value) >= full - 1 else 0
        if run == CLIP_RUN:
            clipped += 1
    return {
        "name": path.name,
        "stem": stem,
        "seconds": round(len(samples) / rate, 2),
        "peakPct": round(peak / full * 100, 1),
        "rmsPct": round(rms / full * 100, 1),
        "dcPct": round(dc / full * 100, 2),
        "clippedRuns": clipped,
        "empty": False,
    }


def problems_in(rows: list[dict]) -> list[str]:
    found = []
    for row in rows:
        silent = row["empty"] or row["peakPct"] == 0.0
        if silent and row["stem"] not in EXPECTED_SILENT:
            found.append(f"{row['name']}：全靜音")
        if row["clippedRuns"]:
            found.append(f"{row['name']}：削波 {row['clippedRuns']} 處")
        if not silent and row["peakPct"] < QUIET_PEAK_PCT:
            found.append(f"{row['name']}：峰值只有 {row['peakPct']}%，幾乎聽不到")
    return found


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=pathlib.Path, help="放轉出 WAV 的目錄")
    parser.add_argument("--output", type=pathlib.Path, help="輸出 JSON 路徑；省略就印到 stdout")
    args = parser.parse_args()

    files = sorted(args.directory.glob("*.wav"))
    if not files:
        raise SystemExit(f"{args.directory} 裡沒有 .wav")
    rows = [analyse(path) for path in files]
    found = problems_in(rows)
    document = {
        "status": "pass" if not found else "fail",
        "note": "客觀健檢，不等於人耳驗收（WORKLIST D3）",
        "fileCount": len(rows),
        "problems": found,
        "expectedSilent": sorted(EXPECTED_SILENT),
        "sounds": rows,
    }
    text = json.dumps(document, ensure_ascii=False, indent=1, sort_keys=True)
    if args.output:
        args.output.write_text(text + "\n", encoding="utf-8")
    else:
        print(text)
    if found:
        sys.exit(1)


if __name__ == "__main__":
    main()
