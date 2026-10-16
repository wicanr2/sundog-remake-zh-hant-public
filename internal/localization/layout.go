package localization

import (
	"strings"
	"unicode"

	"golang.org/x/image/font"
)

// FitText returns one single-line label that fits maxWidth pixels in face.
// It is intentionally presentation-only: it does not alter catalog entries,
// state, save data, or source evidence.
func FitText(face font.Face, value string, maxWidth int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if maxWidth <= 0 || value == "" {
		return ""
	}
	if font.MeasureString(face, value).Ceil() <= maxWidth {
		return value
	}
	const ellipsis = "…"
	ellipsisWidth := font.MeasureString(face, ellipsis).Ceil()
	if ellipsisWidth > maxWidth {
		return ""
	}
	runes := []rune(value)
	for length := len(runes) - 1; length >= 0; length-- {
		candidate := string(runes[:length]) + ellipsis
		if font.MeasureString(face, candidate).Ceil() <= maxWidth {
			return candidate
		}
	}
	return ellipsis
}

// WrapText breaks a localized label into pixel-bounded lines. Explicit
// newlines are preserved, CJK text may break between any two runes, and
// whitespace-delimited text prefers a word boundary. If maxLines is reached,
// the last line receives an ellipsis. This is presentation-only: catalog
// entries, source identities, rules, save data, and evidence are unchanged.
func WrapText(face font.Face, value string, maxWidth, maxLines int) []string {
	if maxWidth <= 0 || maxLines <= 0 || value == "" {
		return nil
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")

	var lines []string
	for _, paragraph := range strings.Split(value, "\n") {
		paragraphRunes := []rune(paragraph)
		if len(paragraphRunes) == 0 {
			lines = append(lines, "")
			continue
		}
		for len(paragraphRunes) > 0 {
			fit := fitRuneCount(face, paragraphRunes, maxWidth)
			if fit == 0 {
				// A single glyph can be wider than the requested column. Keep
				// progress deterministic; callers can still see the glyph.
				fit = 1
			}
			breakAt := fit
			if fit < len(paragraphRunes) {
				for index := fit; index > 0; index-- {
					if unicode.IsSpace(paragraphRunes[index-1]) {
						if index-1 > 0 {
							breakAt = index - 1
						}
						break
					}
				}
			}
			line := strings.TrimRightFunc(string(paragraphRunes[:breakAt]), unicode.IsSpace)
			lines = append(lines, line)
			paragraphRunes = paragraphRunes[breakAt:]
			for len(paragraphRunes) > 0 && unicode.IsSpace(paragraphRunes[0]) {
				paragraphRunes = paragraphRunes[1:]
			}
		}
	}

	if len(lines) <= maxLines {
		return lines
	}
	lines = lines[:maxLines]
	lines[maxLines-1] = appendEllipsis(face, lines[maxLines-1], maxWidth)
	return lines
}

func fitRuneCount(face font.Face, runes []rune, maxWidth int) int {
	for count := 1; count <= len(runes); count++ {
		if font.MeasureString(face, string(runes[:count])).Ceil() > maxWidth {
			return count - 1
		}
	}
	return len(runes)
}

func appendEllipsis(face font.Face, value string, maxWidth int) string {
	const ellipsis = "…"
	if font.MeasureString(face, ellipsis).Ceil() > maxWidth {
		return ""
	}
	if font.MeasureString(face, value+ellipsis).Ceil() <= maxWidth {
		return value + ellipsis
	}
	runes := []rune(value)
	for count := len(runes) - 1; count >= 0; count-- {
		candidate := string(runes[:count]) + ellipsis
		if font.MeasureString(face, candidate).Ceil() <= maxWidth {
			return candidate
		}
	}
	return ellipsis
}
