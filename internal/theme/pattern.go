package theme

import "fmt"

// PixelPattern is a deliberately tiny, reviewable prototype tile. Each ASCII
// digit indexes Group.Palette. It is not decoded original art until a source
// consumer and screen oracle prove the corresponding asset bytes.
type PixelPattern struct {
	Rows []string
}

// PaletteIndex decodes the compact pattern alphabet. Hexadecimal digits keep
// the representation source-free while allowing an external Atari ST
// 16-colour atlas to retain all palette indices.
func PaletteIndex(value byte) (int, bool) {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0'), true
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10, true
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10, true
	default:
		return 0, false
	}
}

func (p PixelPattern) Validate(paletteSize int) error {
	if len(p.Rows) == 0 {
		return fmt.Errorf("pattern has no rows")
	}
	width := len(p.Rows[0])
	if width == 0 {
		return fmt.Errorf("pattern has an empty first row")
	}
	for rowIndex, row := range p.Rows {
		if len(row) != width {
			return fmt.Errorf("row %d has width %d, want %d", rowIndex, len(row), width)
		}
		for columnIndex := 0; columnIndex < len(row); columnIndex++ {
			value, ok := PaletteIndex(row[columnIndex])
			if !ok || value >= paletteSize {
				return fmt.Errorf("pixel (%d,%d) is not a palette index below %d", columnIndex, rowIndex, paletteSize)
			}
		}
	}
	return nil
}

func pattern(rows ...string) PixelPattern {
	return PixelPattern{Rows: rows}
}

func appleIIPatterns() map[string]PixelPattern {
	return map[string]PixelPattern{
		Grass:  pattern("22222222", "22212222", "22222222", "22222212", "22122222", "22222222", "22221222", "22222222"),
		Water:  pattern("01010101", "10101010", "01010101", "10131010", "01010101", "10101010", "01010101", "10101010"),
		Road:   pattern("33333333", "33333333", "33033033", "33333333", "33333333", "33033033", "33333333", "33333333"),
		Wall:   pattern("11111111", "13311331", "11111111", "11331133", "11111111", "13311331", "11111111", "11331133"),
		Ship:   pattern("00011000", "00111100", "01111110", "11111111", "11111111", "01111110", "00111100", "00011000"),
		Cargo:  pattern("00000000", "03333330", "03111130", "03133130", "03111130", "03333330", "00000000", "00000000"),
		City:   pattern("00011000", "00111100", "01111110", "01133110", "11133111", "11111111", "11111111", "11111111"),
		Player: pattern("00011000", "00111100", "00122100", "01111110", "01111110", "00111100", "00100100", "01100110"),
	}
}

func atariSTPatterns() map[string]PixelPattern {
	return map[string]PixelPattern{
		Grass:  pattern("22222222", "22222222", "22022202", "22222222", "22202222", "22222222", "22220222", "22222222"),
		Water:  pattern("02020202", "20202020", "02020202", "20202020", "02020202", "20202020", "02020202", "20202020"),
		Road:   pattern("11111111", "11111111", "11331133", "11111111", "11111111", "13311331", "11111111", "11111111"),
		Wall:   pattern("33333333", "31133113", "33333333", "33113311", "33333333", "31133113", "33333333", "33113311"),
		Ship:   pattern("00011000", "00111100", "01122110", "11222211", "11222211", "01122110", "00111100", "00011000"),
		Cargo:  pattern("00000000", "03333330", "03222230", "03233230", "03222230", "03333330", "00000000", "00000000"),
		City:   pattern("00011000", "00111100", "01111110", "01222110", "12222221", "11111111", "11111111", "11111111"),
		Player: pattern("00011000", "00111100", "00211200", "01111110", "01222210", "00111100", "00100100", "01100110"),
	}
}

func modernApplePatterns() map[string]PixelPattern {
	return redesignModernPatterns(appleIIPatterns())
}
