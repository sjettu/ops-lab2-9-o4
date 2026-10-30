#!/usr/bin/env python3
"""Recreate derived CSVs from archived queries, with original stage timestamps."""
import argparse
import json
import os
from pathlib import Path
from run import loopback, query

p = argparse.ArgumentParser()
p.add_argument("folders", nargs="+", type=Path)
args = p.parse_args()
for folder in args.folders:
    saved = json.loads((folder / "run.json").read_text())
    query(folder.resolve(), loopback(os.environ.get("PROM", saved["prom"])))
    print(f"已重新查询：{folder}；原始日志与阶段记录未修改")
