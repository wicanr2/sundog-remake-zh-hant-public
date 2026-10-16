package localization

import (
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

func TestFitTextRespectsPixelWidth(t *testing.T) {
	face := basicfont.Face7x13
	value := "繁中介面文字與 English status"
	got := FitText(face, value, 24)
	if got == value {
		t.Fatalf("FitText did not truncate %q", value)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("FitText should use an ellipsis, got %q", got)
	}
	if width := font.MeasureString(face, got).Ceil(); width > 24 {
		t.Fatalf("FitText width=%d exceeds limit: %q", width, got)
	}
}

func TestFitTextNormalizesNewlinesAndKeepsShortText(t *testing.T) {
	face := basicfont.Face7x13
	if got := FitText(face, "短\n文字", 200); got != "短 文字" {
		t.Fatalf("FitText newline normalization = %q", got)
	}
	if got := FitText(face, "文字", 0); got != "" {
		t.Fatalf("FitText zero width = %q", got)
	}
}

func TestWrapTextPreservesCJKLinesWithinPixelWidth(t *testing.T) {
	face := basicfont.Face7x13
	lines := WrapText(face, "繁中介面文字與提示", 28, 4)
	if len(lines) < 2 || len(lines) > 4 {
		t.Fatalf("WrapText line count = %d, want 2-4: %#v", len(lines), lines)
	}
	for index, line := range lines {
		if width := font.MeasureString(face, line).Ceil(); width > 28 {
			t.Fatalf("WrapText line %d width=%d exceeds limit: %q", index, width, line)
		}
	}
}

func TestWrapTextPrefersWordsAndEllipsizesOverflow(t *testing.T) {
	face := basicfont.Face7x13
	lines := WrapText(face, "交易所 opens the local bank window", 70, 2)
	if len(lines) != 2 {
		t.Fatalf("WrapText line count = %d, want 2: %#v", len(lines), lines)
	}
	if strings.Contains(lines[0], " ") && strings.HasSuffix(lines[0], " ") {
		t.Fatalf("WrapText left trailing whitespace: %#v", lines)
	}
	if !strings.HasSuffix(lines[1], "…") {
		t.Fatalf("WrapText overflow should end with ellipsis: %#v", lines)
	}
	if width := font.MeasureString(face, lines[1]).Ceil(); width > 70 {
		t.Fatalf("WrapText ellipsis line width=%d exceeds limit: %q", width, lines[1])
	}
}

func TestWrapTextPreservesExplicitBlankLine(t *testing.T) {
	face := basicfont.Face7x13
	lines := WrapText(face, "第一行\n\n第三行", 200, 4)
	want := []string{"第一行", "", "第三行"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("WrapText explicit lines = %#v, want %#v", lines, want)
	}
}
