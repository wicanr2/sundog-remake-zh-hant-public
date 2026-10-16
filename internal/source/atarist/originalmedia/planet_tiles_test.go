package originalmedia

import (
	"os"
	"testing"
)

func TestPlanetStripsFromSuppliedDisk(t *testing.T) {
	path := os.Getenv("SUNDOG_ATARI_DISK")
	if path == "" {
		t.Skip("需提供玩家自備磁碟")
	}
	disk, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for planet := 0; planet < 18; planet++ {
		set, err := disk.LoadPlanetTiles(planet)
		if err != nil {
			t.Fatal(err)
		}
		for _, code := range []int{2, 12, 13, 17, 28, 29, 32, 33, 34, 42} {
			pixels, ok := set.SurfaceTileIndices(code)
			if !ok || len(pixels) != 32*32 {
				t.Fatalf("行星 %d 圖碼 %d 超出圖像邊界", planet, code)
			}
		}
	}
	for _, index := range []int{-1, 18} {
		if _, err := disk.LoadPlanetTiles(index); err == nil {
			t.Fatalf("接受越界行星 %d", index)
		}
	}
}
