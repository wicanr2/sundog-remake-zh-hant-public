from __future__ import annotations

import hashlib
import unittest

from apple2_runtime_code_provenance import (
    CodeSample,
    SECTOR_ORDER_CANDIDATES,
    TRACK_SIZE,
    build_candidate_view,
    build_report,
    match_sample,
    parse_trace,
)


class RuntimeCodeProvenanceTest(unittest.TestCase):
    def sample_line(self, *, start: str = "$9514", length: int = 32, payload: bytes | None = None) -> bytes:
        payload = payload if payload is not None else bytes(range(32))
        return (
            "SUNDOG_HGR_CODE_SAMPLE window=flip-prompt-to-shipboard page=hgr-page-1 "
            f"pc=$951c start={start} length={length} bytes={payload.hex()}\n"
        ).encode("utf-8")

    def test_parse_trace_keeps_only_normalized_identity(self) -> None:
        samples = parse_trace(b"unrelated\n" + self.sample_line())
        self.assertEqual(1, len(samples))
        self.assertEqual(("flip-prompt-to-shipboard", "hgr-page-1", 0x951C), samples[0].identity)
        self.assertEqual(0x9514, samples[0].start)

    def test_parse_trace_rejects_bad_start_or_length(self) -> None:
        with self.assertRaisesRegex(ValueError, "does not precede PC"):
            parse_trace(self.sample_line(start="$9515"))
        with self.assertRaisesRegex(ValueError, "declares"):
            parse_trace(self.sample_line(length=31))

    def test_candidate_view_and_segments_preserve_raw_source(self) -> None:
        payload = bytes(range(32))
        disk = bytearray(TRACK_SIZE)
        # In the DOS 3.3 candidate view logical sector 1 maps to physical 7.
        raw_offset = 7 * 256 + 11
        disk[raw_offset : raw_offset + len(payload)] = payload
        sample = CodeSample("shipboard-to-status", "hgr-page-1", 0xA000, 0x9FF8, payload)
        matches = match_sample(sample, "disk2", bytes(disk))
        dos = [item for item in matches if item["candidateView"] == "dos33-2-descending"]
        self.assertEqual(1, len(dos))
        self.assertEqual("0x0010b", dos[0]["viewOffset"])
        self.assertEqual(raw_offset, dos[0]["sourceSegments"][0]["rawOffset"])
        self.assertEqual(7, dos[0]["sourceSegments"][0]["physicalSector"])
        self.assertEqual(1, dos[0]["sourceSegments"][0]["logicalSector"])

    def test_report_does_not_emit_runtime_bytes(self) -> None:
        payload = bytes(range(32))
        disk = bytearray(TRACK_SIZE)
        disk[17 : 17 + len(payload)] = payload
        trace = self.sample_line(payload=payload)
        report = build_report(trace, "disk1.dsk", bytes(disk), "disk2.dsk", bytes(disk))
        encoded = str(report)
        self.assertNotIn(payload.hex(), encoded)
        self.assertEqual(hashlib.sha256(payload).hexdigest(), report["samples"][0]["runtimeSampleSha256"])
        self.assertIn("raw-sequential", {item["candidateView"] for item in report["samples"][0]["exactByteMatches"]})

    def test_candidate_view_reorders_one_track(self) -> None:
        disk = bytearray(TRACK_SIZE)
        for sector in range(16):
            disk[sector * 256] = sector
        view = build_candidate_view(disk, SECTOR_ORDER_CANDIDATES["apple-pascal-2-ascending"])
        self.assertEqual(0, view[0])
        self.assertEqual(8, view[256])
        self.assertEqual(1, view[512])


if __name__ == "__main__":
    unittest.main()
