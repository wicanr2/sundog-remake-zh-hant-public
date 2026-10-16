package originalmedia

import "testing"

func TestGroundSpritesReadTheTenPatterns(t *testing.T) {
	bytes := make([]byte, 0x5A000)
	walker := []byte{0x00, 0x38, 0x38, 0x38, 0x38, 0x38, 0x38, 0x00}
	copy(bytes[groundSpritesOffset:], walker)
	for index := 1; index < GroundSpriteCount; index++ {
		copy(bytes[groundSpritesOffset+index*groundSpriteStride:], []byte{1, 2, 3, 4, 5, 6, 7, 0})
	}
	disk := &Disk{bytes: bytes}
	patterns, err := disk.GroundSprites()
	if err != nil {
		t.Fatal(err)
	}
	if patterns[0][1] != 0x38 || patterns[9][6] != 7 {
		t.Fatalf("patterns misread: %v", patterns)
	}
}

func TestANonZeroPaddingByteIsRefused(t *testing.T) {
	bytes := make([]byte, 0x5A000)
	copy(bytes[groundSpritesOffset:], []byte{0x00, 0x38, 0x38, 0x38, 0x38, 0x38, 0x38, 0x99})
	disk := &Disk{bytes: bytes}
	if _, err := disk.GroundSprites(); err == nil {
		t.Fatal("a pattern with a non-zero padding byte was accepted")
	}
}

func TestAWrongFirstPatternIsRefused(t *testing.T) {
	// 第 0 樣不是直向人形棒就代表位址讀錯，寧可失敗也不畫錯圖。
	bytes := make([]byte, 0x5A000)
	disk := &Disk{bytes: bytes}
	if _, err := disk.GroundSprites(); err == nil {
		t.Fatal("an all-zero table was accepted")
	}
}
