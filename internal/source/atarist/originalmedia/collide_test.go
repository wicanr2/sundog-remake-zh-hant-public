package originalmedia

import "testing"

// buildCollideTable 造一份合法的取樣圖樣表：第 0 組是 3×6 的載具外框（park_pods 用
// 的那一組），其餘九組各是一個 2×2 的方塊。
func buildCollideTable() []byte {
	bytes := make([]byte, 0x5A000)
	// 第 0 組：起始 y=1、第一個 dx=2，然後沿著外框走十五步。
	first := []byte{1, 2}
	outline := [][2]byte{
		{0, 1}, {0, 1}, // (3,1) (4,1)
		{1, 2}, {0, 2}, // 換行到 (2,2)，再到 (4,2)
		{1, 2}, {0, 2},
		{1, 2}, {0, 2},
		{1, 2}, {0, 2},
		{1, 2}, {0, 1}, {0, 1},
	}
	group0 := append([]byte{}, first...)
	for _, p := range outline {
		group0 = append(group0, p[0], p[1])
	}
	groups := [][]byte{group0}
	for i := 1; i < CollidePatternCount; i++ {
		// 起始 y=3、第一個 dx=3，三步走出 2×2。
		groups = append(groups, []byte{3, 3, 0, 1, 1, 3, 0, 1})
	}
	offsets := make([]int, CollidePatternCount)
	cursor := 0
	for i, g := range groups {
		offsets[i] = cursor
		cursor += len(g)
	}
	for i, off := range offsets {
		bytes[collideOffsetsOffset+i*2] = byte(off >> 8)
		bytes[collideOffsetsOffset+i*2+1] = byte(off & 0xff)
		bytes[collideCountsOffset+i] = byte((len(groups[i]) - 2) / 2)
	}
	for i, g := range groups {
		copy(bytes[collideDataOffset+offsets[i]:], g)
	}
	return bytes
}

func TestCollidePatternsReadTheTenGroups(t *testing.T) {
	disk := &Disk{bytes: buildCollideTable()}
	patterns, err := disk.CollidePatterns()
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != CollidePatternCount {
		t.Fatalf("read %d patterns", len(patterns))
	}
	if width, height := patternExtent(patterns[0]); width != 3 || height != 6 {
		t.Fatalf("pattern 0 is %d×%d, want the 3×6 vehicle outline", width, height)
	}
	if width, height := patternExtent(patterns[6]); width != 2 || height != 2 {
		t.Fatalf("pattern 6 is %d×%d, want the 2×2 stand-in", width, height)
	}
}

// TestCollidePatternsRefuseABrokenChain: 每組佔 2 + 2×筆數 位元組，下一組緊接著開始。
// 這條鏈斷掉就代表位址錯了或磁碟不同，要 fail-closed。
func TestCollidePatternsRefuseABrokenChain(t *testing.T) {
	bytes := buildCollideTable()
	bytes[collideOffsetsOffset+3] += 4 // 把第 1 組的偏移推開
	disk := &Disk{bytes: bytes}
	if _, err := disk.CollidePatterns(); err == nil {
		t.Fatal("偏移鏈斷掉還是被接受了")
	}
}

// TestCollidePatternsRefuseAWrongPatternZero: 第 0 組必須是 3×6 的載具外框——那是
// park_pods 用 collide(0,…) 判的那一組，形狀對不上就不是這張表。
func TestCollidePatternsRefuseAWrongPatternZero(t *testing.T) {
	bytes := buildCollideTable()
	bytes[collideDataOffset+1] = 9 // 把第 0 組的第一個 dx 改掉，寬度就不再是 3
	disk := &Disk{bytes: bytes}
	if _, err := disk.CollidePatterns(); err == nil {
		t.Fatal("第 0 組形狀不對還是被接受了")
	}
}

// TestCollideAnchorIsFour: collide 對 x 與 y 各減 4，重製版的錨點偏移要跟著它。
func TestCollideAnchorIsFour(t *testing.T) {
	if CollideAnchorOffset != 4 {
		t.Fatalf("錨點偏移是 %d，collide 減的是 4", CollideAnchorOffset)
	}
}
