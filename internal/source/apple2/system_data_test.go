package apple2

import "testing"

func TestParseFixedNameTablePreservesRecordsAndUnknownTail(t *testing.T) {
	data := []byte{
		0x03, 'J', 'o', 'n', 0xaa, 0xbb, 0xcc,
		0x04, 'F', 'e', 'r', 'r', 0x01, 0x02,
	}
	table, err := ParseFixedNameTable(data, "systems", 0, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	if table.ObservationLevel != EvidenceProven || table.SemanticLevel != EvidenceUnknown || table.ConsumerLevel != EvidenceUnknown {
		t.Fatalf("unexpected evidence levels: %+v", table)
	}
	if table.Entries[0].RawOffset != 0 || table.Entries[0].Name != "Jon" || table.Entries[0].TailHex != "aabbcc" {
		t.Fatalf("first record lost raw identity: %+v", table.Entries[0])
	}
	if table.Entries[1].RawOffset != 7 || table.Entries[1].Name != "Ferr" || table.Entries[1].TailHex != "0102" {
		t.Fatalf("second record parsed incorrectly: %+v", table.Entries[1])
	}
}

func TestParseFixedNameTableRejectsNonPrintableName(t *testing.T) {
	data := []byte{0x02, 'A', 0x00, 0x00, 0x00}
	if _, err := ParseFixedNameTable(data, "bad", 0, len(data), 1); err == nil {
		t.Fatal("expected non-printable name error")
	}
}
