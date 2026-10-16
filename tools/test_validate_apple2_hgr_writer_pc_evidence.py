from __future__ import annotations

import hashlib
import tempfile
import unittest
from pathlib import Path

from validate_apple2_hgr_writer_pc_evidence import validate, writer_record_lines


class HGRWriterPCEvidenceValidationTest(unittest.TestCase):
    def make_log(self, *, writes: int = 7) -> bytes:
        return (
            "SUNDOG_HGR_WRITER_DISK_SWAP frame=4300 ok=true result=pass\n"
            "SUNDOG_HGR_WRITER_SAMPLE window=shipboard-to-status page=hgr-page-1 pc=$a000 "
            "address=$2000 data=00 mask=00ff\n"
            "SUNDOG_HGR_WRITER_SUMMARY window=shipboard-to-status page=hgr-page-1 "
            "range=$2000..$3fff hardwareRole=standard hi-res page 1 writerCount=1 writes="
            f"{writes}\n"
            "SUNDOG_HGR_WRITER window=shipboard-to-status page=hgr-page-1 pc=$a000 "
            f"writes={writes} firstAddress=$2000 lastAddress=$2006 sampleData=00 sampleMask=00ff\n"
            "SUNDOG_HGR_WRITER_TAP_REMOVE index=1 ok=true result=nil\n"
            "SUNDOG_HGR_WRITER_TRACE_COMPLETE frame=5900\n"
        ).encode("utf-8")

    def make_document(self, disk1: bytes, disk2: bytes, script: bytes, log: bytes, artifact: bytes) -> dict:
        summaries = [
            {
                "window": "shipboard-to-status",
                "page": "hgr-page-1",
                "range": "$2000..$3fff",
                "writerCount": 1,
                "writes": 7,
            }
        ]
        page1 = [
            {
                "window": "shipboard-to-status",
                "page": "hgr-page-1",
                "pc": "$a000",
                "writes": 7,
                "firstAddress": "$2000",
                "lastAddress": "$2006",
                "sampleData": "00",
                "sampleMask": "00ff",
            }
        ]
        run = {
            "log": "mame.log",
            "logSha256": hashlib.sha256(log).hexdigest(),
            "screenshotsOutsideRepository": [
                {"name": "snapshot.bin", "size": len(artifact), "sha256": hashlib.sha256(artifact).hexdigest()}
            ],
        }
        return {
            "input": {
                "disk1": {"size": len(disk1), "sha256": hashlib.sha256(disk1).hexdigest()},
                "disk2": {"size": len(disk2), "sha256": hashlib.sha256(disk2).hexdigest()},
            },
            "replay": {"scriptSha256": hashlib.sha256(script).hexdigest()},
            "trace": {
                "writerRowCount": 1,
                "writerRecordSha256": hashlib.sha256(("\n".join(writer_record_lines(log)) + "\n").encode("utf-8")).hexdigest(),
                "tapRemovalCount": 1,
                "completionFrame": 5900,
            },
            "writerSummaries": summaries,
            "page1WriterRows": page1,
            "traceRuns": [{"name": "primary", **run}, {"name": "repeat", **run}],
        }

    def validate_fixture(self, *, script: bytes = b"trace-script", log: bytes | None = None, artifact: bytes = b"png") -> list[str]:
        disk1 = b"disk one"
        disk2 = b"disk two"
        log = self.make_log() if log is None else log
        document = self.make_document(disk1, disk2, script, log, artifact)
        with tempfile.TemporaryDirectory() as temp_dir:
            primary = Path(temp_dir) / "primary"
            repeat = Path(temp_dir) / "repeat"
            for directory in (primary, repeat):
                directory.mkdir()
                (directory / "mame.log").write_bytes(log)
                (directory / "snapshot.bin").write_bytes(artifact)
            return validate(disk1, disk2, script, document, primary, repeat)

    def test_accepts_matching_runs(self) -> None:
        self.assertEqual([], self.validate_fixture())

    def test_rejects_script_drift(self) -> None:
        disk1 = b"disk one"
        disk2 = b"disk two"
        expected_script = b"trace-script"
        actual_script = b"drifted-script"
        log = self.make_log()
        document = self.make_document(disk1, disk2, expected_script, log, b"png")
        with tempfile.TemporaryDirectory() as temp_dir:
            primary = Path(temp_dir) / "primary"
            repeat = Path(temp_dir) / "repeat"
            for directory in (primary, repeat):
                directory.mkdir()
                (directory / "mame.log").write_bytes(log)
                (directory / "snapshot.bin").write_bytes(b"png")
            self.assertIn("trace script sha256 mismatch", validate(disk1, disk2, actual_script, document, primary, repeat))

    def test_rejects_writer_drift_after_log_hash_refresh(self) -> None:
        disk1 = b"disk one"
        disk2 = b"disk two"
        script = b"trace-script"
        expected_log = self.make_log()
        drifted_log = self.make_log(writes=8)
        document = self.make_document(disk1, disk2, script, expected_log, b"png")
        for run in document["traceRuns"]:
            run["logSha256"] = hashlib.sha256(drifted_log).hexdigest()
        with tempfile.TemporaryDirectory() as temp_dir:
            primary = Path(temp_dir) / "primary"
            repeat = Path(temp_dir) / "repeat"
            for directory in (primary, repeat):
                directory.mkdir()
                (directory / "mame.log").write_bytes(drifted_log)
                (directory / "snapshot.bin").write_bytes(b"png")
            errors = validate(disk1, disk2, script, document, primary, repeat)
            self.assertIn("writer summary mismatch: primary", errors)
            self.assertIn("page 1 writer rows mismatch: repeat", errors)

    def test_rejects_repeat_artifact_drift(self) -> None:
        disk1 = b"disk one"
        disk2 = b"disk two"
        script = b"trace-script"
        log = self.make_log()
        document = self.make_document(disk1, disk2, script, log, b"expected")
        with tempfile.TemporaryDirectory() as temp_dir:
            primary = Path(temp_dir) / "primary"
            repeat = Path(temp_dir) / "repeat"
            for directory in (primary, repeat):
                directory.mkdir()
                (directory / "mame.log").write_bytes(log)
                (directory / "snapshot.bin").write_bytes(b"expected")
            (repeat / "snapshot.bin").write_bytes(b"drifted")
            self.assertIn("screenshot artifact sha256 mismatch: repeat/snapshot.bin", validate(disk1, disk2, script, document, primary, repeat))


if __name__ == "__main__":
    unittest.main()
