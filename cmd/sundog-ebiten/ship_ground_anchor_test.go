package main

import "testing"

// 原版 XREADMAP 0x0739（raw 0x36139）寫入 (17,20)，XMOVEONG 0x0274 將它加到
// 格原點；來源與 Talos 收據見 docs/atari-st/ship-position-20260908.md。
// 期望值來自原版錨點，不以圖示寬高反推置中公式。
func TestGroundShipAnchorsMatchOriginalLandingWriter(t *testing.T) {
	for _, tc := range []struct {
		left, top, noseX, noseY, tailX int
	}{
		{0, 0, 17, 20, 23},
		{160, 80, 177, 100, 183},
		{120, 80, 137, 100, 143},
		{280, 160, 297, 180, 303},
	} {
		marks := stGroundShipMarks(tc.left, tc.top)
		if len(marks) != 2 || marks[0] != (stGroundMark{ch: 11, x: tc.noseX, y: tc.noseY}) ||
			marks[1] != (stGroundMark{ch: 12, x: tc.tailX, y: tc.noseY}) {
			t.Fatalf("格原點 (%d,%d)：船位 %v 不符原版", tc.left, tc.top, marks)
		}
	}
}
