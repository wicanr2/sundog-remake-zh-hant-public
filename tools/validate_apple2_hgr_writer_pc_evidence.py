#!/usr/bin/env python3
"""驗證 Apple II HGR writer-PC trace 的唯讀輸入、log、畫面與可重播摘要。"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any


SUMMARY_RE = re.compile(
    r"^SUNDOG_HGR_WRITER_SUMMARY window=(?P<window>\S+) page=(?P<page>\S+) "
    r"range=(?P<range>\$[0-9a-f]{4}\.\.\$[0-9a-f]{4}) hardwareRole=standard hi-res page [12] "
    r"writerCount=(?P<writerCount>\d+) writes=(?P<writes>\d+)$"
)
WRITER_RE = re.compile(
    r"^SUNDOG_HGR_WRITER window=(?P<window>\S+) page=(?P<page>\S+) pc=(?P<pc>\$[0-9a-f]{4}|unknown) "
    r"writes=(?P<writes>\d+) firstAddress=(?P<firstAddress>\$[0-9a-f]{4}) "
    r"lastAddress=(?P<lastAddress>\$[0-9a-f]{4}) sampleData=(?P<sampleData>[0-9a-f]{2}) "
    r"sampleMask=(?P<sampleMask>[0-9a-f]{4})$"
)
REMOVE_RE = re.compile(r"^SUNDOG_HGR_WRITER_TAP_REMOVE index=(?P<index>\d+) ok=true result=nil$")
COMPLETE_RE = re.compile(r"^SUNDOG_HGR_WRITER_TRACE_COMPLETE frame=(?P<frame>\d+)$")
DISK_SWAP_RE = re.compile(r"^SUNDOG_HGR_WRITER_DISK_SWAP frame=4300 ok=true result=pass$")


def sha256(payload: bytes) -> str:
    return hashlib.sha256(payload).hexdigest()


def writer_record_lines(payload: bytes) -> list[str]:
    prefixes = (
        "SUNDOG_HGR_WRITER_SAMPLE",
        "SUNDOG_HGR_WRITER_SUMMARY",
        "SUNDOG_HGR_WRITER window=",
        "SUNDOG_HGR_WRITER_TAP_REMOVE",
        "SUNDOG_HGR_WRITER_TRACE_COMPLETE",
    )
    return [line for line in payload.decode("utf-8", errors="replace").splitlines() if line.startswith(prefixes)]


def parse_log(payload: bytes) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[int], list[int], bool]:
    summaries: list[dict[str, Any]] = []
    writers: list[dict[str, Any]] = []
    removals: list[int] = []
    completions: list[int] = []
    disk_swap = False
    for line in payload.decode("utf-8", errors="replace").splitlines():
        summary = SUMMARY_RE.match(line)
        if summary:
            item = summary.groupdict()
            item["writerCount"] = int(item["writerCount"])
            item["writes"] = int(item["writes"])
            summaries.append(item)
            continue
        writer = WRITER_RE.match(line)
        if writer:
            item = writer.groupdict()
            item["writes"] = int(item["writes"])
            writers.append(item)
            continue
        removal = REMOVE_RE.match(line)
        if removal:
            removals.append(int(removal.group("index")))
            continue
        complete = COMPLETE_RE.match(line)
        if complete:
            completions.append(int(complete.group("frame")))
            continue
        if DISK_SWAP_RE.match(line):
            disk_swap = True
    return summaries, writers, removals, completions, disk_swap


def validate_run(document: dict[str, Any], run: dict[str, Any], trace_dir: Path) -> list[str]:
    errors: list[str] = []
    name = run.get("name", "unknown")
    log_name = run.get("log")
    if not isinstance(log_name, str) or not log_name:
        return [f"trace log name is missing: {name}"]
    log_path = trace_dir / log_name
    if not log_path.is_file():
        return [f"trace log is missing: {name}/{log_name}"]
    log = log_path.read_bytes()
    if run.get("logSha256") != sha256(log):
        errors.append(f"trace log sha256 mismatch: {name}")

    for artifact in run.get("screenshotsOutsideRepository", []):
        if not isinstance(artifact, dict):
            errors.append(f"screenshot artifact is not an object: {name}")
            continue
        artifact_name = artifact.get("name")
        if not isinstance(artifact_name, str) or not artifact_name:
            errors.append(f"screenshot artifact name is missing: {name}")
            continue
        artifact_path = trace_dir / artifact_name
        if not artifact_path.is_file():
            errors.append(f"screenshot artifact is missing: {name}/{artifact_name}")
            continue
        artifact_payload = artifact_path.read_bytes()
        if artifact.get("sha256") != sha256(artifact_payload):
            errors.append(f"screenshot artifact sha256 mismatch: {name}/{artifact_name}")
        if artifact.get("size") != len(artifact_payload):
            errors.append(f"screenshot artifact size mismatch: {name}/{artifact_name}")

    summaries, writers, removals, completions, disk_swap = parse_log(log)
    expected_summaries = document.get("writerSummaries", [])
    if summaries != expected_summaries:
        errors.append(f"writer summary mismatch: {name}")

    expected_page1 = document.get("page1WriterRows", [])
    page1 = [item for item in writers if item.get("page") == "hgr-page-1"]
    if page1 != expected_page1:
        errors.append(f"page 1 writer rows mismatch: {name}")

    trace = document.get("trace", {})
    if len(writers) != trace.get("writerRowCount"):
        errors.append(f"writer row count mismatch: {name}")
    normalized = ("\n".join(writer_record_lines(log)) + "\n").encode("utf-8")
    if sha256(normalized) != trace.get("writerRecordSha256"):
        errors.append(f"writer record sha256 mismatch: {name}")
    expected_removals = list(range(1, trace.get("tapRemovalCount", 0) + 1))
    if removals != expected_removals:
        errors.append(f"tap removal sequence mismatch: {name}")
    if completions != [trace.get("completionFrame")]:
        errors.append(f"trace completion frame mismatch: {name}")
    if not disk_swap:
        errors.append(f"disk swap observation is missing: {name}")
    return errors


def validate(
    disk1: bytes,
    disk2: bytes,
    script: bytes,
    document: dict[str, Any],
    primary_trace_dir: Path,
    repeat_trace_dir: Path,
) -> list[str]:
    errors: list[str] = []
    for key, payload in (("disk1", disk1), ("disk2", disk2)):
        input_info = document.get("input", {}).get(key, {})
        if input_info.get("size") != len(payload):
            errors.append(f"{key} size mismatch")
        if input_info.get("sha256") != sha256(payload):
            errors.append(f"{key} sha256 mismatch")

    if document.get("replay", {}).get("scriptSha256") != sha256(script):
        errors.append("trace script sha256 mismatch")

    runs = document.get("traceRuns", [])
    if not isinstance(runs, list) or len(runs) != 2:
        return [*errors, "traceRuns must contain primary and repeat runs"]
    errors.extend(validate_run(document, runs[0], primary_trace_dir))
    errors.extend(validate_run(document, runs[1], repeat_trace_dir))
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("disk1", type=Path)
    parser.add_argument("disk2", type=Path)
    parser.add_argument("script", type=Path)
    parser.add_argument("evidence", type=Path)
    parser.add_argument("primary_trace_dir", type=Path)
    parser.add_argument("repeat_trace_dir", type=Path)
    args = parser.parse_args()

    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    errors = validate(
        args.disk1.read_bytes(),
        args.disk2.read_bytes(),
        args.script.read_bytes(),
        document,
        args.primary_trace_dir,
        args.repeat_trace_dir,
    )
    result = {
        "status": "pass" if not errors else "fail",
        "errors": errors,
        "page1WriterCount": len(document.get("page1WriterRows", [])),
        "summaryCount": len(document.get("writerSummaries", [])),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
