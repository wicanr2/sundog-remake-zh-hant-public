package localization

import "testing"

func TestSupportedLocalesAndFallback(t *testing.T) {
	want := []Locale{TraditionalChinese, SimplifiedChinese, Japanese, English}
	got := Locales()
	if len(got) != len(want) {
		t.Fatalf("locale count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] || !KnownLocale(got[index]) {
			t.Fatalf("locale[%d] = %q, want %q", index, got[index], want[index])
		}
	}
	if got := NormalizeLocale(""); got != TraditionalChinese {
		t.Fatalf("empty locale normalized to %q", got)
	}
	if got := NormalizeLocale("unknown"); got != TraditionalChinese {
		t.Fatalf("unknown locale normalized to %q", got)
	}
}
