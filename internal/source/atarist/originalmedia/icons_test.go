package originalmedia

import "testing"

// stripOf builds a strip of n cells where cell i is filled with palette index
// i, so a mis-cut shows up as a cell holding a neighbour's colour.
func stripOf(cells int) Picture {
	width := cells * ItemIconSize
	pixels := make([]byte, width*ItemIconSize)
	for row := 0; row < ItemIconSize; row++ {
		for index := 0; index < cells; index++ {
			for column := 0; column < ItemIconSize; column++ {
				pixels[row*width+index*ItemIconSize+column] = byte(index & 0x0F)
			}
		}
	}
	return Picture{Width: width, Height: ItemIconSize, Pixels: pixels}
}

func TestEachItemIconCellKeepsItsOwnColumn(t *testing.T) {
	cells, err := sliceItemIcons(stripOf(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 3 {
		t.Fatalf("cut %d cells, want 3", len(cells))
	}
	colours := PaletteRGBA()
	// Cell 0 is palette index 0, the background, so it comes out empty.
	if _, _, _, alpha := cells[0].At(4, 4).RGBA(); alpha != 0 {
		t.Fatalf("the background cell is not transparent: alpha=%d", alpha)
	}
	for index := 1; index < 3; index++ {
		if got, want := cells[index].RGBAAt(0, 0), colours[index]; got != want {
			t.Fatalf("cell %d top-left = %v, want %v", index, got, want)
		}
		if got, want := cells[index].RGBAAt(ItemIconSize-1, ItemIconSize-1), colours[index]; got != want {
			t.Fatalf("cell %d bottom-right = %v, want %v", index, got, want)
		}
	}
}

func TestAStripThatIsNotOneCellTallIsRefused(t *testing.T) {
	picture := stripOf(2)
	picture.Height = ItemIconSize * 2
	if _, err := sliceItemIcons(picture); err == nil {
		t.Fatal("a two-row strip was accepted")
	}
}

func TestAStripWidthThatDoesNotDivideIsRefused(t *testing.T) {
	picture := stripOf(2)
	picture.Width++
	if _, err := sliceItemIcons(picture); err == nil {
		t.Fatal("a strip whose width does not divide was accepted")
	}
}

func TestIconLookupIsByItemCodeNotByCellIndex(t *testing.T) {
	cells, err := sliceItemIcons(stripOf(4))
	if err != nil {
		t.Fatal(err)
	}
	icons := &ItemIcons{first: FirstItemIconCode, cells: cells}
	if _, ok := icons.Icon(0); ok {
		t.Fatal("item code 0 is (nothing) and must have no icon")
	}
	colours := PaletteRGBA()
	// Item code 2 is the second cell, which stripOf filled with palette 1.
	icon, ok := icons.Icon(2)
	if !ok {
		t.Fatal("item code 2 has no icon")
	}
	if got, want := icon.RGBAAt(0, 0), colours[1]; got != want {
		t.Fatalf("item code 2 drew %v, want the second cell's %v", got, want)
	}
	if _, ok := icons.Icon(len(cells) + FirstItemIconCode); ok {
		t.Fatal("a code past the strip returned an icon")
	}
}
