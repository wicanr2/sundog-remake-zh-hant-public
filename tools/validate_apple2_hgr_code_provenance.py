#!/usr/bin/env python3
"""驗證 Apple II HGR writer PC 與唯讀 DSK code-provenance trace。"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any

try:
    from tools.apple2_runtime_code_provenance import build_report, parse_trace, sha256
except ModuleNotFoundError:  # direct ``python tools/...`` invocation
    from apple2_runtime_code_provenance import build_report, parse_trace, sha256


RECORD_PREFIXES = (
    "SUNDOG_HGR_CODE_COUNT",
    "SUNDOG_HGR_CODE_SAMPLE",
    "SUNDOG_HGR_CODE_TAP_REMOVE",
    "SUNDOG_HGR_CODE_TRACE_COMPLETE",
    "SUNDOG_HGR_CODE_DISK_SWAP",
)
SAMPLE_PREFIX = "SUNDOG_HGR_CODE_SAMPLE"
TAP_REMOVE_RE = re.compile(r"^SUNDOG_HGR_CODE_TAP_REMOVE index=(?P<index>\d+) ok=true result=nil$")
COMPLETE_RE = re.compile(r"^SUNDOG_HGR_CODE_TRACE_COMPLETE frame=(?P<frame>\d+)$")
DISK_SWAP = "SUNDOG_HGR_CODE_DISK_SWAP frame=4300 ok=true result=pass"


def normalized_lines(payload: bytes, prefixes: tuple[str, ...] = RECORD_PREFIXES) -> list[str]:
    return [
        line
        for line in payload.decode("utf-8", errors="replace").splitlines()
        if line.startswith(prefixes)
    ]


def normalized_sha256(lines: list[str]) -> str:
    return sha256(("\n".join(lines) + "\n").encode("utf-8"))


def compact_matched_samples(report: dict[str, object]) -> list[dict[str, object]]:
    """Drop runtime bytes and candidate-view duplicate rows from a provenance report."""

    result: list[dict[str, object]] = []
    for row in report["samples"]:
        assert isinstance(row, dict)
        matches = row["exactByteMatches"]
        assert isinstance(matches, list)
        if not matches:
            continue
        locations: dict[tuple[str, str], dict[str, object]] = {}
        for match in matches:
            assert isinstance(match, dict)
            segments = match["sourceSegments"]
            assert isinstance(segments, list)
            raw_segments = [
                {
                    key: segment[key]
                    for key in ("track", "physicalSector", "rawOffset", "rawEndExclusive", "length")
                }
                for segment in segments
            ]
            key = (str(match["disk"]), json.dumps(raw_segments, sort_keys=True, separators=(",", ":")))
            location = locations.get(key)
            if location is None:
                location = {
                    "disk": match["disk"],
                    "sourceSegments": raw_segments,
                    "candidateViews": [],
                }
                locations[key] = location
            location["candidateViews"].append(
                {
                    "view": match["candidateView"],
                    "logicalSectors": [segment["logicalSector"] for segment in segments],
                }
            )
        compact_locations = []
        for location in locations.values():
            location["candidateViews"] = sorted(
                location["candidateViews"],
                key=lambda item: (str(item["view"]), list(item["logicalSectors"])),
            )
            compact_locations.append(location)
        compact_locations.sort(
            key=lambda item: (
                str(item["disk"]),
                json.dumps(item["sourceSegments"], sort_keys=True, separators=(",", ":")),
            )
        )
        result.append(
            {
                "window": row["window"],
                "page": row["page"],
                "pc": row["pc"],
                "runtimeStart": row["runtimeStart"],
                "runtimeLength": row["runtimeLength"],
                "runtimeSampleSha256": row["runtimeSampleSha256"],
                "matchingSourceLocations": compact_locations,
            }
        )
    return result


def trace_contract(payload: bytes) -> dict[str, object]:
    records = normalized_lines(payload)
    samples = [line for line in records if line.startswith(SAMPLE_PREFIX)]
    removals: list[int] = []
    completions: list[int] = []
    for line in records:
        removal = TAP_REMOVE_RE.match(line)
        if removal:
            removals.append(int(removal.group("index")))
        completion = COMPLETE_RE.match(line)
        if completion:
            completions.append(int(completion.group("frame")))
    return {
        "recordCount": len(records),
        "recordSha256": normalized_sha256(records),
        "sampleCount": len(samples),
        "sampleRecordSha256": normalized_sha256(samples),
        "tapRemovals": removals,
        "completionFrames": completions,
        "diskSwapObserved": DISK_SWAP in records,
    }


def validate_run(
    document: dict[str, Any],
    run: dict[str, Any],
    trace_dir: Path,
    disk1_name: str,
    disk1: bytes,
    disk2_name: str,
    disk2: bytes,
) -> tuple[list[str], list[dict[str, object]]]:
    errors: list[str] = []
    name = str(run.get("name", "unknown"))
    log_name = run.get("log")
    if not isinstance(log_name, str) or not log_name:
        return [f"trace log name is missing: {name}"], []
    log_path = trace_dir / log_name
    if not log_path.is_file():
        return [f"trace log is missing: {name}/{log_name}"], []
    payload = log_path.read_bytes()
    if run.get("logSha256") != sha256(payload):
        errors.append(f"trace log sha256 mismatch: {name}")

    contract = trace_contract(payload)
    expected = document.get("trace", {})
    for key in ("recordCount", "recordSha256", "sampleCount", "sampleRecordSha256"):
        if contract[key] != expected.get(key):
            errors.append(f"trace {key} mismatch: {name}")
    expected_removals = list(range(1, int(expected.get("tapRemovalCount", 0)) + 1))
    if contract["tapRemovals"] != expected_removals:
        errors.append(f"tap removal sequence mismatch: {name}")
    if contract["completionFrames"] != [expected.get("completionFrame")]:
        errors.append(f"trace completion frame mismatch: {name}")
    if not contract["diskSwapObserved"]:
        errors.append(f"disk swap observation is missing: {name}")

    try:
        parse_trace(payload)
    except ValueError as error:
        errors.append(f"invalid code sample trace: {name}: {error}")
        return errors, []
    report = build_report(payload, disk1_name, disk1, disk2_name, disk2)
    summary = report["summary"]
    if summary != expected.get("summary"):
        errors.append(f"match summary mismatch: {name}")
    compact = compact_matched_samples(report)
    if compact != document.get("matchedSamples", []):
        errors.append(f"matched source locations mismatch: {name}")
    return errors, compact


def validate(
    disk1_name: str,
    disk1: bytes,
    disk2_name: str,
    disk2: bytes,
    script: bytes,
    document: dict[str, Any],
    primary_trace_dir: Path,
    repeat_trace_dir: Path,
) -> list[str]:
    errors: list[str] = []
    inputs = document.get("input", {})
    for key, payload in (("disk1", disk1), ("disk2", disk2)):
        expected = inputs.get(key, {})
        if expected.get("size") != len(payload):
            errors.append(f"{key} size mismatch")
        if expected.get("sha256") != sha256(payload):
            errors.append(f"{key} sha256 mismatch")
    if document.get("replay", {}).get("scriptSha256") != sha256(script):
        errors.append("trace script sha256 mismatch")

    runs = document.get("traceRuns", [])
    if not isinstance(runs, list) or [run.get("name") for run in runs] != ["primary", "repeat"]:
        return [*errors, "traceRuns must contain primary then repeat"]
    primary_errors, primary_compact = validate_run(
        document, runs[0], primary_trace_dir, disk1_name, disk1, disk2_name, disk2
    )
    repeat_errors, repeat_compact = validate_run(
        document, runs[1], repeat_trace_dir, disk1_name, disk1, disk2_name, disk2
    )
    errors.extend(primary_errors)
    errors.extend(repeat_errors)
    if primary_compact != repeat_compact:
        errors.append("primary and repeat matched source locations differ")
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
    disk1 = args.disk1.read_bytes()
    disk2 = args.disk2.read_bytes()
    errors = validate(
        args.disk1.name,
        disk1,
        args.disk2.name,
        disk2,
        args.script.read_bytes(),
        document,
        args.primary_trace_dir,
        args.repeat_trace_dir,
    )
    print(
        json.dumps(
            {
                "status": "pass" if not errors else "fail",
                "errors": errors,
                "matchedSampleCount": len(document.get("matchedSamples", [])),
                "sampleCount": document.get("trace", {}).get("sampleCount"),
            },
            ensure_ascii=False,
            indent=2,
            sort_keys=True,
        )
    )
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
