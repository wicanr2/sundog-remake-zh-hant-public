import hashlib
import os
from pathlib import Path
import unittest
from unittest.mock import patch
import atari_st_colony_phases as phases

class ColonyPhasesTest(unittest.TestCase):
    def test_unknown_input_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "SHA-256"):
            phases.decode(b"not a disk")

    @unittest.skipUnless(os.environ.get("SUNDOG_ATARI_DISK"), "需要本機原始磁碟")
    def test_original_phase_requirements(self):
        disk = Path(os.environ["SUNDOG_ATARI_DISK"]).read_bytes()
        result = phases.decode(disk)
        rows = result["phases"]
        self.assertEqual(len(rows), 9)
        self.assertEqual(rows[0]["goods"], [0,0,0,0,8,9,11,13])
        self.assertEqual([r["requiredCounts"][31] for r in rows], [0,2,4,1,2,3,2,2,1])
        self.assertEqual(rows[-1]["goods"], [2,7,5,24,26,28,29,31])
        self.assertTrue(all(sum(r["requiredCounts"]) == sum(g != 0 for g in r["goods"]) for r in rows))
        # 即使外層 hash 閘門被測試替換，仍須檢查消費端，不能只吃表。
        changed = bytearray(disk)
        changed[phases.BASE+0x16A5] = 0x41
        with patch.object(phases, "DISK_SHA256", hashlib.sha256(changed).hexdigest()):
            with self.assertRaisesRegex(ValueError, "消費指令"):
                phases.decode(bytes(changed))

if __name__ == "__main__":
    unittest.main()
