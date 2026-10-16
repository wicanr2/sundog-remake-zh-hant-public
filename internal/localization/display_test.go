package localization

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/catalog"
)

func TestPlaceLookupUsesOverlayAndFallback(t *testing.T) {
	labels := catalog.Catalog{
		Locale: "zh-Hant",
		Entries: map[string]catalog.Entry{
			"ui.destination.jond-banville": {Key: "ui.destination.jond-banville", Translation: "Jond[d]／Banville（候選目標）"},
		},
	}
	if got := Destination(labels, "jond-banville", "raw target"); got != "Jond[d]／Banville（候選目標）" {
		t.Fatalf("destination overlay = %q", got)
	}
	if got := System(labels, "unknown", "raw system"); got != "raw system" {
		t.Fatalf("system fallback = %q", got)
	}
}

func TestLibrarySlotLabelHidesStoragePrefix(t *testing.T) {
	if got := LibrarySlotLabel("slot-1"); got != "1" {
		t.Fatalf("slot label = %q, want 1", got)
	}
	if got := LibrarySlotLabel("custom"); got != "custom" {
		t.Fatalf("custom slot label = %q", got)
	}
}
