"""致命當拍收據的合成護欄；入口docs/atari-st/ship-rules.md，不計原版案數。"""
import copy
import unittest

from verify_talos_combat_death import FIELDS, compare


class LethalParentGateTests(unittest.TestCase):
    def setUp(self):
        self.native = []
        self.formal = []
        for enemy in range(7):
            for mode in range(8):
                for phase in range(3):
                    for hull in (1, 2, 3, 4, 16):
                        for seed in (0, 1, 17, 41):
                            for layout in (0, 1):
                                data = dict(Enemy=enemy, Mode=mode, Phase=phase, Hull=hull, Seed=seed, Layout=layout)
                                systems = [[0]*18 for _ in range(6)]
                                statuses = [16, 16, 8, 16, 8, 16] if layout == 0 else [0, 16, 0, 0, 0, 0]
                                for s, status in zip(systems, statuses):
                                    s[16] = status
                                step = {field: 0 for field in FIELDS}
                                dead = data == dict(Enemy=0, Mode=0, Phase=0, Hull=1, Seed=0, Layout=1)
                                step.update(Phase=(phase+1)%3, Hull=0 if dead else hull, Counter=seed+1,
                                            Systems=copy.deepcopy(systems), Words=[0, 0], Active=True,
                                            Flags=[0, 0, 1 << 12, 0, 0], RandomWords=[seed+1, 0], Ready=False)
                                step.pop("CanPrepareFire")
                                row = dict(Input=data, InitialSystems=systems, Step=step,
                                           Global3=1 if dead else 0, DisplayMode=12 if dead else 7, Cause=2 if dead else 0,
                                           Calls=[[0x31e00, at, proc] for at, proc in ((0x1295, 0x1f), (0x1298, 0x23), (0x12a8, 0x21))],
                                           Draws=[dict(Low=0, High=1, Value=0, Counter=seed+1)])
                                self.native.append(row)
                                formal_step = {field: copy.deepcopy(step[field]) if field != "CanPrepareFire" else True
                                               for field in FIELDS}
                                formal_step["Active"] = not dead
                                self.formal.append(dict(Input=copy.deepcopy(data), InitialSystems=copy.deepcopy(systems),
                                                        Step=formal_step, Screen="outcome" if dead else "tactical",
                                                        Outcome="ui.outcome.failure" if dead else "", SleepActive=False))
        self.death_index = next(i for i, r in enumerate(self.native) if r["Global3"] == 1)

    def test_complete_pair_passes_only_scoped(self):
        r = compare(self.native, self.formal)
        self.assertEqual(r["status"], "PASS-scoped")
        self.assertEqual(r["deaths"], 1)
        self.assertEqual(r["cases"], 6720)
        self.assertEqual(r["originalFullDeathGUI"], "unverified")

    def test_death_after_extra_roll_is_rejected(self):
        self.formal[self.death_index]["Step"]["Counter"] += 1
        r = compare(self.native, self.formal)
        self.assertEqual(r["status"], "FAIL")
        self.assertEqual(r["differences"]["Counter"], 1)

    def test_death_after_extra_system_damage_is_rejected(self):
        self.formal[self.death_index]["Step"]["Systems"][1][0] += 128
        r = compare(self.native, self.formal)
        self.assertEqual(r["status"], "FAIL")
        self.assertEqual(r["differences"]["Systems"], 1)

    def test_no_lethal_cases_cannot_pass(self):
        a, b = self.native[self.death_index], self.formal[self.death_index]
        a.update(Global3=0, DisplayMode=7, Cause=0)
        a["Step"]["Hull"] = b["Step"]["Hull"] = 1
        b.update(Screen="tactical", Outcome="")
        b["Step"]["Active"] = True
        self.assertEqual(compare(self.native, self.formal)["status"], "FAIL")

    def test_original_encounter_bit_cannot_be_relabelled(self):
        self.native[self.death_index]["Step"]["Active"] = False
        self.assertEqual(compare(self.native, self.formal)["status"], "FAIL")

    def test_wrong_death_cause_is_rejected(self):
        self.native[self.death_index]["Cause"] = 1
        r = compare(self.native, self.formal)
        self.assertEqual(r["status"], "FAIL")
        self.assertEqual(r["differences"]["Terminal"], 1)

    def test_missing_initial_systems_on_both_sides_is_rejected(self):
        self.native[0].pop("InitialSystems")
        self.formal[0].pop("InitialSystems")
        self.assertEqual(compare(self.native, self.formal)["status"], "FAIL")

    def test_missing_parent_call_is_rejected(self):
        self.native[0]["Calls"].pop()
        self.assertEqual(compare(self.native, self.formal)["status"], "FAIL")

    def test_duplicate_input_is_rejected(self):
        self.native[0]["Input"] = copy.deepcopy(self.native[1]["Input"])
        self.assertEqual(compare(self.native, self.formal)["status"], "FAIL")


if __name__ == "__main__":
    unittest.main()
