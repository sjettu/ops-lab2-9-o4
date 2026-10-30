#!/usr/bin/env python3
"""Lab2 collection workflow adapted to textstats; Linux, Python standard library."""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import time
import urllib.request
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
HTTP = urllib.request.build_opener(urllib.request.ProxyHandler({}))
SLO = "0.02"


def fetch(url, body=None):
    req = urllib.request.Request(url, data=body)
    if body is not None:
        req.add_header("Content-Type", "application/json")
    with HTTP.open(req, timeout=5) as response:
        return response.read()


def command(args):
    return subprocess.check_output(args, cwd=ROOT, text=True, timeout=10).strip()


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def cpu_set(value):
    if not re.fullmatch(r"\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*", value):
        raise ValueError("CPU 列表格式应为 0 或 0,2 或 0-2")
    result = set()
    for part in value.split(","):
        ends = [int(x) for x in part.split("-")]
        if ends[0] > ends[-1]:
            raise ValueError("CPU 范围起点不能大于终点")
        result.update(range(ends[0], ends[-1] + 1))
    return result


def loopback(value):
    parsed = urlparse(value)
    if (parsed.scheme != "http" or parsed.hostname not in {"127.0.0.1", "localhost"}
            or parsed.username or parsed.password or parsed.path not in {"", "/"}
            or parsed.query or parsed.fragment):
        raise ValueError("服务与监控地址必须是本机 http://127.0.0.1:端口")
    return value.rstrip("/")


def validate_request(path):
    body = path.read_bytes()
    if len(body) > 1024 * 1024 or b"{{seq}}" in body:
        raise ValueError("固定请求不能超过 1 MiB，也不能包含 {{seq}} 模板")
    obj = json.loads(body)
    if (not isinstance(obj, dict) or set(obj) != {"text"}
            or not isinstance(obj["text"], str) or not obj["text"].strip()):
        raise ValueError('请求必须为 {"text":"非空文本"}')
    return body


def query(folder, prom):
    meta = json.loads((folder / "run.json").read_text())
    args = [str(ROOT / "bin/promsteps"), "-steps", str(folder / "steps.csv"), "-prom", prom]
    for key, value in meta["variables"].items():
        args += ["-var", f"{key}={value}"]
    for kind in ["red", "use"]:
        dest = folder / f"{kind}.csv"
        temporary = dest.with_suffix(".csv.new")
        try:
            with temporary.open("w") as output:
                subprocess.run(args + ["-queries", str(folder / f"queries-{kind}.txt")],
                               cwd=ROOT, stdout=output, check=True, timeout=180)
            temporary.replace(dest)
        finally:
            temporary.unlink(missing_ok=True)


def run(name, request_path, rates):
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", name):
        raise ValueError("实验名只能包含字母、数字、点、下划线或短横线")
    numbers = [float(x) for x in rates.split(",")]
    if not numbers or any(not math.isfinite(x) or x < 0 for x in numbers) or not any(numbers):
        raise ValueError("到达率应为逗号分隔的非负有限数，且至少一阶段有负载")
    kind = os.environ.get("RUN_KIND", "formal")
    if kind not in {"formal", "integration"}:
        raise ValueError("RUN_KIND 只能为 formal 或 integration")
    if os.environ.get("SLO", SLO) != SLO:
        raise ValueError("本实验固定 SLO=0.02 秒；请求超时是另一项参数")
    step = int(os.environ.get("STEP_SECONDS", "30"))
    if step < 20 or (kind == "formal" and step < 30):
        raise ValueError("正式阶段至少30秒，联调阶段至少20秒")
    if kind == "formal" and ("microsoft" in platform.release().lower() or Path("/.dockerenv").exists()):
        raise ValueError("WSL/容器仅用于联调；云主机上的正式测量请保持 RUN_KIND=formal")
    svc = loopback(os.environ.get("SVC", "http://127.0.0.1:18080"))
    prom = loopback(os.environ.get("PROM", "http://127.0.0.1:19090"))
    generator = cpu_set(os.environ.get("GEN_CPUS", "0"))
    if len(generator) != 1 or not generator <= os.sched_getaffinity(0):
        raise ValueError("负载发生器应绑定一个当前可用的逻辑核")
    os.sched_setaffinity(0, generator)
    for tool in ["vmstat", "mpstat", "iostat", "pidstat", "taskset"]:
        if not shutil.which(tool):
            raise ValueError(f"缺少 {tool}；vmstat 属于 procps，其余采集器属于 sysstat")
    for tool in ["textstats", "loadgen", "promsteps"]:
        if not os.access(ROOT / "bin" / tool, os.X_OK):
            raise ValueError("请先在新项目根目录 make build")
    body = validate_request(request_path)
    out_root = Path(os.environ.get("OUT_ROOT", str(ROOT.parent / "ops-lab2-9/runs"))).resolve()
    folder = out_root / name
    if folder.exists():
        raise ValueError(f"{folder} 已存在，请使用新的实验名，不覆盖原始数据")
    version = json.loads(fetch(svc + "/version"))
    if version.get("name") != "textstats" or version.get("gomaxprocs") != 1:
        raise ValueError("服务应为 textstats 且 GOMAXPROCS=1；请绑定一个逻辑核启动")
    pid = int(version["pid"])
    proc = Path(f"/proc/{pid}")
    if ((proc / "cwd").resolve() != ROOT
            or (proc / "exe").resolve() != ROOT / "bin/textstats"):
        raise ValueError("服务进程必须从本机当前新项目目录启动，且二进制未被替换")
    service = os.sched_getaffinity(pid)
    if len(service) != 1 or service & generator:
        raise ValueError("服务和负载发生器必须各绑定一个不同的逻辑核")
    commit = command(["git", "rev-parse", "HEAD"])
    dirty = command(["git", "status", "--porcelain"])
    if kind == "formal" and (dirty or version.get("commit") != commit):
        raise ValueError("正式测量前需提交代码并重新构建，确保服务提交号等于干净的仓库 HEAD")
    targets = json.loads(fetch(prom + "/api/v1/targets"))["data"]["activeTargets"]
    for job in ["textstats", "node"]:
        selected = [t for t in targets if t["labels"].get("job") == job]
        if len(selected) != 1 or selected[0]["health"] != "up":
            raise ValueError(f"Prometheus 应恰有一个健康的 {job} 目标")
        if job == "textstats" and selected[0]["scrapeUrl"] != svc + "/metrics":
            raise ValueError("Prometheus 抓取的业务服务不是本轮服务")
    if "textstats_process_metrics_supported 1" not in fetch(svc + "/metrics").decode():
        raise ValueError("当前服务未提供 Linux 进程 CPU 指标")
    # A real business response checks the input before starting the measured stages.
    response = fetch(svc + "/api/analyze", body)
    result = json.loads(response)
    if result.get("word_count", 0) <= 0 or result.get("unique_words", 0) <= 0:
        raise ValueError("固定请求未得到有效词频统计结果")
    out_root.mkdir(parents=True, exist_ok=True)
    folder.mkdir()  # Existing runs are never overwritten.
    (folder / "request.json").write_bytes(body)
    (folder / "sample-response.json").write_bytes(response)
    for key in ["red", "use"]:
        shutil.copyfile(ROOT / f"deploy/queries-{key}.txt", folder / f"queries-{key}.txt")
    variables = {"route": "/api/analyze", "slo": SLO,
                 "cpus": "|".join(map(str, sorted(service)))}
    generator_str = ",".join(map(str, sorted(generator)))
    load = ["taskset", "-c", generator_str, str(ROOT / "bin/loadgen"),
            "-url", svc + "/api/analyze", "-method", "POST", "-body-file", str(folder / "request.json"),
            "-rates", rates, "-step", f"{step}s", "-timeout", "2s", "-slo", "0.02s",
            "-max-inflight", "20000", "-seed", "1", "-out", str(folder / "steps.csv")]
    meta = {"kind": kind, "name": name, "started": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
            "host": platform.uname()._asdict(), "commit": commit, "dirty": dirty,
            "version": version, "service_cpus": sorted(service), "generator_cpus": sorted(generator),
            "svc": svc, "prom": prom, "variables": variables, "command": load,
            "request_sha256": sha(folder / "request.json"),
            "binaries_sha256": {x: sha(ROOT / "bin" / x) for x in ["textstats", "loadgen", "promsteps"]},
            "queries_sha256": {x: sha(folder / f"queries-{x}.txt") for x in ["red", "use"]}}
    (folder / "run.json").write_text(json.dumps(meta, indent=2, ensure_ascii=False) + "\n")
    (folder / "meta.txt").write_text(
        f"类型: {kind}（integration 不能作为正式性能结果）\n名字: {name}\n"
        f"开始: {meta['started']}\n主机: {platform.platform()}\n"
        f"服务提交: {version['commit']}；仓库提交: {commit}\n"
        f"服务绑核: {sorted(service)}；loadgen 绑核: {sorted(generator)}\n"
        f"接口: /api/analyze；SLO 阈值 0.02s；请求超时 2s\n"
        f"请求文件 SHA256: {meta['request_sha256']}\n"
        f"promsteps 变量: {' '.join(k+'='+v for k,v in variables.items())}\n"
        f"压测命令: {json.dumps(load, ensure_ascii=False)}\n")
    (folder / "metrics-start.txt").write_bytes(fetch(svc + "/metrics"))
    collectors = [["vmstat", "-t", "-w", "1"], ["mpstat", "-P", "ALL", "1"],
                  ["iostat", "-d", "-x", "-t", "1"], ["pidstat", "-h", "-u", "-r", "-w", "-p", str(pid), "1"]]
    children, handles = [], []
    env = dict(os.environ, LC_ALL="C", S_TIME_FORMAT="ISO", GOMAXPROCS="1")
    print(f"检查通过：{kind}；SLO=20 ms；到达率={rates}；数据目录={folder}", flush=True)
    try:
        for args in collectors:
            output = (folder / f"{args[0]}.log").open("wb")
            handles.append(output)
            children.append(subprocess.Popen(args, stdout=output, stderr=subprocess.STDOUT, env=env))
        time.sleep(0.3)
        if any(p.poll() is not None for p in children):
            raise RuntimeError("资源采集器启动失败，详见原始日志")
        print("负载运行中，逐阶段输出保存在 loadgen.log。", flush=True)
        with (folder / "loadgen.log").open("wb") as output:
            subprocess.run(load, cwd=ROOT, stdout=output, stderr=subprocess.STDOUT,
                           env=env, check=True, timeout=step * len(numbers) + 30)
        time.sleep(3)  # Let scrapes covering the final stage arrive.
        if any(p.poll() is not None for p in children):
            raise RuntimeError("资源采集器提前退出，原始日志保留，不能视作完整实验")
        (folder / "metrics-end.txt").write_bytes(fetch(svc + "/metrics"))
    finally:
        for child in children:
            if child.poll() is None:
                child.terminate()
        for child in children:
            try:
                child.wait(timeout=3)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait(timeout=3)
        for handle in handles:
            handle.close()
    query(folder, prom)
    print((folder / "loadgen.log").read_text(), end="")
    print(f"完成：{folder}。请检查三张CSV中的错误、等待、CPU和发送滞后。")


def main():
    parser = argparse.ArgumentParser(description="textstats 固定20ms实验采集；不覆盖已有目录")
    parser.add_argument("name")
    parser.add_argument("request", type=Path)
    parser.add_argument("rates")
    args = parser.parse_args()
    try:
        run(args.name, args.request.resolve(), args.rates)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f"采集未完成：{error}。已生成的原始文件保留。", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
