#!/usr/bin/env python3
"""以 FFmpeg 文字圖層產生獨立故事卡，不使用手冊掃描當畫面。"""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess

from PIL import ImageFont


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--story", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--font", type=Path, required=True)
    args = parser.parse_args()
    story = json.loads(args.story.read_text())
    assert not args.output.exists()
    args.output.mkdir(parents=True)
    scenes, checks = [], []
    for index, card in enumerate(story["cards"]):
        layers = [("精訊資訊《陽犬號2.0版》故事背景", 28, 90), (card["heading"], 58, 190)]
        layers += [(line, 40, 330 + line_index * 80) for line_index, line in enumerate(card["lines"])]
        layers += [("故事依精訊資訊《陽犬號2.0版》手冊第 4–9 頁改寫", 22, 710)]
        filters = ["drawbox=x=40:y=40:w=1200:h=720:color=0x6ed9bb:t=2"]
        for line_index, (text, size, y) in enumerate(layers):
            font = ImageFont.truetype(str(args.font), size)
            width = font.getlength(text)
            assert width <= 1120 and y + size <= 760, (text, width)
            path = args.output / f"card-{index}-line-{line_index}.txt"
            path.write_text(text)
            colour = "0x70e0af" if line_index == 0 else "white"
            filters.append(f"drawtext=fontfile={args.font}:textfile={path}:fontsize={size}:fontcolor={colour}:x=80:y={y}")
            checks.append({"text": text, "fontSize": size, "width": width, "safeWidth": 1120, "y": y})
        image = args.output / f"story-{index + 1}.png"
        subprocess.run(["ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=0x102943:s=1280x800:r=30", "-vf", ",".join(filters), "-frames:v", "1", "-threads", "1", str(image)], check=True, timeout=30)
        scenes.append({"source": str(image), "seconds": card["seconds"], "caption": "故事背景｜精訊資訊《陽犬號2.0版》手冊改寫"})
    (args.output / "scenes.json").write_text(json.dumps(scenes, ensure_ascii=False, indent=2) + "\n")
    (args.output / "validation.json").write_text(json.dumps({"status": "pass", "sourceSHA256": hashlib.sha256(args.story.read_bytes()).hexdigest(), "fontSHA256": hashlib.sha256(args.font.read_bytes()).hexdigest(), "textBounds": checks, "scope": "independent text cards; original scans excluded"}, ensure_ascii=False, indent=2) + "\n")
    print("PASS: three story cards and all text bounds")


if __name__ == "__main__":
    main()
