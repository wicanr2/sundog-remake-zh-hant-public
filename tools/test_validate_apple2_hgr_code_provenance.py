from __future__ import annotations

import hashlib
import tempfile
import unittest
from pathlib import Path

from apple2_runtime_code_provenance import build_report
from validate_apple2_hgr_code_provenance import (
    compact_matched_samples,
    normalized_lines,
    normalized_sha256,
    trace_contract,
    validate,
)


class HGRCodeProvenanceValidationTest(unittest.TestCase):
    payload = bytes(range(32))

    def make_log(self, payload: bytes | None = None) -> bytes:
        payload = payload if payload is not None else self.payload
        return (
            "SUNDOG_HGR_CODE_DISK_SWAP frame=4300 ok=true result=pass\n"
            "SUNDOG_HGR_CODE_COUNT window=shipboard-to-status page=hgr-page-1 writes=1\n"
            "SUNDOG_HGR_CODE_SAMPLE window=shipboard-to-status page=hgr-page-1 "
            f"pc=$951c start=$9514 length={len(payload)} bytes={payload.hex()}\n"
            "SUNDOG_HGR_CODE_TAP_REMOVE index=1 ok=true result=nil\n"
            "SUNDOG_HGR_CODE_TAP_REMOVE index=2 ok=true result=nil\n"
            "SUNDOG_HGR_CODE_TRACE_COMPLETE frame=5900\n"
        ).encode("utf-8")

    def make_disk(self, payload: bytes | None = None) -> bytes:
        payload = payload if payload is not None else self.payload
        disk = bytearray(4096)
        disk[21 : 21 + len(payload)] = payload
        return bytes(disk)

    def make_document(self, disk1: bytes, disk2: bytes, script: bytes, log: bytes) -> dict:
        report = build_report(log, "disk1.dsk", disk1, "disk2.dsk", disk2)
        contract = trace_contract(log)
        return {
            "input": {
                "disk1": {"size": len(disk1), "sha256": hashlib.sha256(disk1).hexdigest()},
                "disk2": {"size": len(disk2), "sha256": hashlib.sha256(disk2).hexdigest()},
            },
            "replay": {"scriptSha256": hashlib.sha256(script).hexdigest()},
            "trace": {
                "recordCount": contract["recordCount"],
                "recordSha256": contract["recordSha256"],
                "sampleCount": contract["sampleCount"],
                "sampleRecordSha256": contract["sampleRecordSha256"],
                "tapRemovalCount": 2,
                "completionFrame": 5900,
                "summary": report["summary"],
            },
            "matchedSamples": compact_matched_samples(report),
            "traceRuns": [
                {"name": "primary", "log": "mame.log", "logSha256": hashlib.sha256(log).hexdigest()},
                {"name": "repeat", "log": "mame.log", "logSha256": hashlib.sha256(log).hexdigest()},
            ],
        }

    def validate_fixture(self, *, script: bytes = b"trace", log: bytes | None = None) -> list[str]:
        log = self.make_log() if log is None else log
        disk1 = self.make_disk()
        disk2 = self.make_disk()
        document = self.make_document(disk1, disk2, script, log)
        with tempfile.TemporaryDirectory() as temp_dir:
            primary = Path(temp_dir) / "primary"
            repeat = Path(temp_dir) / "repeat"
            for directory in (primary, repeat):
                directory.mkdir()
                (directory / "mame.log").write_bytes(log)
            return validate("disk1.dsk", disk1, "disk2.dsk", disk2, script, document, primary, repeat)

    def test_accepts_matching_runs(self) -> None:
        self.assertEqual([], self.validate_fixture())

    def test_rejects_script_drift(self) -> None:
        log = self.make_log()
        disk = self.make_disk()
        document = self.make_document(disk, disk, b"trace", log)
        with tempfile.TemporaryDirectory() as temp_dir:
            primary = Path(temp_dir) / "primary"
            repeat = Path(temp_dir) / "repeat"
            for directory in (primary, repeat):
                directory.mkdir()
                (directory / "mame.log").write_bytes(log)
            errors = validate("disk1.dsk", disk, "disk2.dsk", disk, b"drift", document, primary, repeat)
        self.assertIn("trace script sha256 mismatch", errors)

    def test_rejects_code_sample_drift_when_log_hash_is_refreshed(self) -> None:
        expected = self.make_log()
        drifted = self.make_log(self.payload[:-1] + b"\xff")
        disk = self.make_disk()
        document = self.make_document(disk, disk, b"trace", expected)
        for run in document["traceRuns"]:
            run["logSha256"] = hashlib.sha256(drifted).hexdigest()
        with tempfile.TemporaryDirectory() as temp_dir:
            primary = Path(temp_dir) / "primary"
            repeat = Path(temp_dir) / "repeat"
            for directory in (primary, repeat):
                directory.mkdir()
                (directory / "mame.log").write_bytes(drifted)
            errors = validate("disk1.dsk", disk, "disk2.dsk", disk, b"trace", document, primary, repeat)
        self.assertIn("trace sampleRecordSha256 mismatch: primary", errors)
        self.assertIn("matched source locations mismatch: repeat", errors)

    def test_record_normalization_ignores_non_contract_noise(self) -> None:
        log = b"Fontconfig noise\n" + self.make_log() + b"MAME pointer 0x1234\n"
        self.assertEqual(trace_contract(self.make_log()), trace_contract(log))
        self.assertEqual(
            normalized_sha256(normalized_lines(self.make_log())),
            normalized_sha256(normalized_lines(log)),
        )


if __name__ == "__main__":
    unittest.main()
