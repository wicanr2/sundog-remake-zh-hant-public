from __future__ import annotations

import hashlib
import tempfile
import unittest
from pathlib import Path

from validate_apple2_display_page_evidence import validate


class DisplayPageEvidenceValidationTest(unittest.TestCase):
    def make_log(self, writes: int = 7) -> bytes:
        return (
            "SUNDOG_DISPLAY_PAGE window=shipboard-to-status key=hgr-page-1 "
            "range=$2000..$3fff hardwareRole=standard hi-res page 1 "
            f"writes={writes} firstWrite=$2000 lastWrite=$2006 nonzero=5 "
            "firstNonzero=$2000 lastNonzero=$2004 sum=00000005 weightedSum=0000000f\n"
            "SUNDOG_DISPLAY_SWITCH_SUMMARY window=shipboard-to-status key=graphics address=$c050 reads=0 writes=0\n"
            "SUNDOG_DISPLAY_TAP_REMOVE index=1 ok=true result=nil\n"
            "SUNDOG_DISPLAY_TRACE_COMPLETE frame=5900\n"
        ).encode("utf-8")

    def make_document(self, disk1: bytes, disk2: bytes, script: bytes, log: bytes, artifact: bytes) -> dict:
        return {
            "input": {
                "disk1": {"size": len(disk1), "sha256": hashlib.sha256(disk1).hexdigest()},
                "disk2": {"size": len(disk2), "sha256": hashlib.sha256(disk2).hexdigest()},
            },
            "replay": {"scriptSha256": hashlib.sha256(script).hexdigest()},
            "trace": {
                "log": "mame.log",
                "logSha256": hashlib.sha256(log).hexdigest(),
                "completionFrame": 5900,
                "tapRemovalCount": 1,
            },
            "traceArtifactsOutsideRepository": [
                {"name": "snapshot.bin", "size": len(artifact), "sha256": hashlib.sha256(artifact).hexdigest()}
            ],
            "observedPageWindows": [
                {
                    "window": "shipboard-to-status",
                    "page": "hgr-page-1",
                    "writes": 7,
                    "firstWrite": "$2000",
                    "lastWrite": "$2006",
                    "nonzero": 5,
                    "sum": "00000005",
                    "weightedSum": "0000000f",
                }
            ],
            "softSwitchObservation": {"summaryLineCount": 1, "readCount": 0, "writeCount": 0},
        }

    def validate_fixture(self, *, script: bytes = b"trace-script", log: bytes | None = None, artifact: bytes = b"png") -> list[str]:
        disk1 = b"disk one"
        disk2 = b"disk two"
        log = self.make_log() if log is None else log
        document = self.make_document(disk1, disk2, script, log, artifact)
        with tempfile.TemporaryDirectory() as temp_dir:
            trace_dir = Path(temp_dir)
            (trace_dir / "mame.log").write_bytes(log)
            (trace_dir / "snapshot.bin").write_bytes(artifact)
            return validate(disk1, disk2, script, document, trace_dir)

    def test_accepts_matching_trace(self) -> None:
        self.assertEqual([], self.validate_fixture())

    def test_rejects_script_drift(self) -> None:
        disk1 = b"disk one"
        disk2 = b"disk two"
        expected_script = b"trace-script"
        actual_script = b"drifted-script"
        log = self.make_log()
        artifact = b"png"
        document = self.make_document(disk1, disk2, expected_script, log, artifact)
        with tempfile.TemporaryDirectory() as temp_dir:
            trace_dir = Path(temp_dir)
            (trace_dir / "mame.log").write_bytes(log)
            (trace_dir / "snapshot.bin").write_bytes(artifact)
            self.assertIn("trace script sha256 mismatch", validate(disk1, disk2, actual_script, document, trace_dir))

    def test_rejects_page_write_drift(self) -> None:
        disk1 = b"disk one"
        disk2 = b"disk two"
        script = b"trace-script"
        expected_log = self.make_log()
        actual_log = self.make_log(writes=8)
        artifact = b"png"
        document = self.make_document(disk1, disk2, script, expected_log, artifact)
        document["trace"]["logSha256"] = hashlib.sha256(actual_log).hexdigest()
        with tempfile.TemporaryDirectory() as temp_dir:
            trace_dir = Path(temp_dir)
            (trace_dir / "mame.log").write_bytes(actual_log)
            (trace_dir / "snapshot.bin").write_bytes(artifact)
            self.assertIn("page observation mismatch: shipboard-to-status/hgr-page-1 writes", validate(disk1, disk2, script, document, trace_dir))

    def test_rejects_artifact_hash_drift(self) -> None:
        disk1 = b"disk one"
        disk2 = b"disk two"
        script = b"trace-script"
        log = self.make_log()
        document = self.make_document(disk1, disk2, script, log, b"expected")
        with tempfile.TemporaryDirectory() as temp_dir:
            trace_dir = Path(temp_dir)
            (trace_dir / "mame.log").write_bytes(log)
            (trace_dir / "snapshot.bin").write_bytes(b"drifted")
            self.assertIn("trace artifact sha256 mismatch: snapshot.bin", validate(disk1, disk2, script, document, trace_dir))


if __name__ == "__main__":
    unittest.main()
