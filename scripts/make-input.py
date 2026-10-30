#!/usr/bin/env python3
"""Create a fixed, deterministic business request; do not overwrite a file."""
import argparse
import json
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument("output", type=Path)
p.add_argument("--text-bytes", type=int, default=65536)
args = p.parse_args()
if not 1024 <= args.text_bytes <= 900000:
    p.error("text-bytes 必须在 1024～900000 之间")
words = " ".join(f"word{i:04d}" for i in range(1024)) + " "
text = (words * (args.text_bytes // len(words) + 1))[:args.text_bytes]
args.output.parent.mkdir(parents=True, exist_ok=True)
with args.output.open("x", encoding="utf-8") as f:
    json.dump({"text": text}, f, separators=(",", ":"))
print(f"已生成 {args.output}，文本 {len(text.encode())} 字节")
