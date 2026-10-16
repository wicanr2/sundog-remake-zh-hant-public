#!/usr/bin/env python3
"""驗證 Apple II 顯示頁面 trace 的輸入、腳本、log 與 /tmp artifact 證據。"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any


PAGE_RE = re.compile(
    r"^SUNDOG_DISPLAY_PAGE window=(?P<window>\S+) key=(?P<page>\S+) "
    r"range=(?P<range>\$[0-9a-f]{4}\.\.\$[0-9a-f]{4}) hardwareRole=(?P<role>.+?) "
    r"writes=(?P<writes>\d+) firstWrite=(?P<firstWrite>\S+) lastWrite=(?P<lastWrite>\S+) "
    r"nonzero=(?P<nonzero>\d+) firstNonzero=(?P<firstNonzero>\S+) lastNonzero=(?P<lastNonzero>\S+) "
    r"sum=(?P<sum>[0-9a-f]{8}) weightedSum=(?P<weightedSum>[0-9a-f]{8})$"
)
SWITCH_RE = re.compile(
    r"^SUNDOG_DISPLAY_SWITCH_SUMMARY window=(?P<window>\S+) key=(?P<key>\S+) "
    r"address=\$[0-9a-f]{4} reads=(?P<reads>\d+) writes=(?P<writes>\d+)$"
)
REMOVE_RE = re.compile(r"^SUNDOG_DISPLAY_TAP_REMOVE index=(?P<index>\d+) ok=true result=nil$")
COMPLETE_RE = re.compile(r"^SUNDOG_DISPLAY_TRACE_COMPLETE frame=(?P<frame>\d+)$")


def sha256(payload: bytes) -> str:
    return hashlib.sha256(payload).hexdigest()


def parse_log(payload: bytes) -> tuple[dict[tuple[str, str], dict[str, Any]], list[dict[str, Any]], list[int], list[int]]:
    pages: dict[tuple[str, str], dict[str, Any]] = {}
    switches: list[dict[str, Any]] = []
    removals: list[int] = []
    completions: list[int] = []
    for line in payload.decode("utf-8", errors="replace").splitlines():
        page = PAGE_RE.match(line)
        if page:
            item = page.groupdict()
            item["writes"] = int(item["writes"])
            item["nonzero"] = int(item["nonzero"])
            pages[(item["window"], item["page"])] = item
            continue
        switch = SWITCH_RE.match(line)
        if switch:
            item = switch.groupdict()
            item["reads"] = int(item["reads"])
            item["writes"] = int(item["writes"])
            switches.append(item)
            continue
        removal = REMOVE_RE.match(line)
        if removal:
            removals.append(int(removal.group("index")))
            continue
        complete = COMPLETE_RE.match(line)
        if complete:
            completions.append(int(complete.group("frame")))
    return pages, switches, removals, completions


def validate(disk1: bytes, disk2: bytes, script: bytes, document: dict[str, Any], trace_dir: Path) -> list[str]:
    errors: list[str] = []
    input_info = document.get("input", {})
    for key, payload in (("disk1", disk1), ("disk2", disk2)):
        info = input_info.get(key, {})
        if info.get("size") != len(payload):
            errors.append(f"{key} size mismatch")
        if info.get("sha256") != sha256(payload):
            errors.append(f"{key} sha256 mismatch")

    replay = document.get("replay", {})
    if replay.get("scriptSha256") != sha256(script):
        errors.append("trace script sha256 mismatch")

    trace = document.get("trace", {})
    log_name = trace.get("log")
    if not isinstance(log_name, str) or not log_name:
        return [*errors, "trace log name is missing"]
    log_path = trace_dir / log_name
    if not log_path.is_file():
        return [*errors, f"trace log is missing: {log_name}"]
    log = log_path.read_bytes()
    if trace.get("logSha256") != sha256(log):
        errors.append("trace log sha256 mismatch")

    for artifact in document.get("traceArtifactsOutsideRepository", []):
        if not isinstance(artifact, dict):
            errors.append("trace artifact is not an object")
            continue
        name = artifact.get("name")
        if not isinstance(name, str) or not name:
            errors.append("trace artifact name is missing")
            continue
        path = trace_dir / name
        if not path.is_file():
            errors.append(f"trace artifact is missing: {name}")
            continue
        payload = path.read_bytes()
        if artifact.get("sha256") != sha256(payload):
            errors.append(f"trace artifact sha256 mismatch: {name}")
        if "size" in artifact and artifact["size"] != len(payload):
            errors.append(f"trace artifact size mismatch: {name}")

    pages, switches, removals, completions = parse_log(log)
    observations = document.get("observedPageWindows", [])
    if not isinstance(observations, list) or not observations:
        errors.append("observedPageWindows is empty or not a list")
    for expected in observations:
        if not isinstance(expected, dict):
            errors.append("page observation is not an object")
            continue
        window = expected.get("window")
        page = expected.get("page")
        actual = pages.get((window, page))
        if actual is None:
            errors.append(f"page observation is missing: {window}/{page}")
            continue
        for field in ("writes", "firstWrite", "lastWrite", "nonzero", "sum", "weightedSum"):
            if actual.get(field) != expected.get(field):
                errors.append(f"page observation mismatch: {window}/{page} {field}")

    switch_expectation = document.get("softSwitchObservation", {})
    expected_lines = switch_expectation.get("summaryLineCount")
    if expected_lines != len(switches):
        errors.append(f"soft-switch summary count mismatch: {len(switches)}")
    if sum(item["reads"] for item in switches) != switch_expectation.get("readCount"):
        errors.append("soft-switch read count mismatch")
    if sum(item["writes"] for item in switches) != switch_expectation.get("writeCount"):
        errors.append("soft-switch write count mismatch")

    if trace.get("tapRemovalCount") != len(removals) or removals != list(range(1, len(removals) + 1)):
        errors.append("tap removal sequence mismatch")
    completion_frame = trace.get("completionFrame")
    if completions != [completion_frame]:
        errors.append("trace completion frame mismatch")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("disk1", type=Path)
    parser.add_argument("disk2", type=Path)
    parser.add_argument("script", type=Path)
    parser.add_argument("evidence", type=Path)
    parser.add_argument("trace_dir", type=Path)
    args = parser.parse_args()

    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    errors = validate(
        args.disk1.read_bytes(),
        args.disk2.read_bytes(),
        args.script.read_bytes(),
        document,
        args.trace_dir,
    )
    result = {
        "status": "pass" if not errors else "fail",
        "errors": errors,
        "pageObservationCount": len(document.get("observedPageWindows", [])),
        "traceArtifactCount": len(document.get("traceArtifactsOutsideRepository", [])),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
