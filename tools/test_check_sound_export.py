#!/usr/bin/env python3
"""check_sound_export 的判準測試：用合成的 WAV，不需要原版磁碟。"""

import math
import pathlib
import struct
import tempfile
import unittest
import wave

from check_sound_export import EXPECTED_SILENT, analyse, problems_in


def write_wav(path: pathlib.Path, samples: list[int], rate: int = 22050) -> None:
    with wave.open(str(path), "wb") as handle:
        handle.setnchannels(1)
        handle.setsampwidth(2)
        handle.setframerate(rate)
        handle.writeframes(b"".join(struct.pack("<h", v) for v in samples))


class SoundCheckTests(unittest.TestCase):
    def rows_for(self, name: str, samples: list[int]) -> list[dict]:
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / name
            write_wav(path, samples)
            return [analyse(path)]

    def test_a_clean_tone_passes(self) -> None:
        tone = [int(12000 * math.sin(i / 8)) for i in range(2000)]
        self.assertEqual(problems_in(self.rows_for("01-tone.wav", tone)), [])

    def test_three_samples_at_full_scale_count_as_clipping(self) -> None:
        clipped = [32767, 32767, 32767] + [0] * 100
        found = problems_in(self.rows_for("02-clip.wav", clipped))
        self.assertTrue(any("削波" in message for message in found), found)

    def test_two_samples_at_full_scale_do_not(self) -> None:
        # A square wave touches full scale constantly; the run length is what
        # separates a chip's own output from a botched conversion.
        edge = [32767, 32767, 0, 0] * 50
        self.assertEqual(problems_in(self.rows_for("03-edge.wav", edge)), [])

    def test_silence_is_reported_unless_the_table_expects_it(self) -> None:
        silence = [0] * 500
        found = problems_in(self.rows_for("09-something.wav", silence))
        self.assertTrue(any("全靜音" in message for message in found), found)
        expected = problems_in(self.rows_for("00-off.wav", silence))
        self.assertEqual(expected, [])
        self.assertIn("00-off", EXPECTED_SILENT)

    def test_direct_offset_alone_is_not_a_problem(self) -> None:
        # A square chip's output sits off centre; that is not a defect.
        offset = [8000] * 400 + [0] * 400
        self.assertEqual(problems_in(self.rows_for("04-dc.wav", offset)), [])

    def test_a_barely_audible_file_is_reported(self) -> None:
        whisper = [80 if i % 2 else -80 for i in range(500)]
        found = problems_in(self.rows_for("05-quiet.wav", whisper))
        self.assertTrue(any("幾乎聽不到" in message for message in found), found)


if __name__ == "__main__":
    unittest.main()
