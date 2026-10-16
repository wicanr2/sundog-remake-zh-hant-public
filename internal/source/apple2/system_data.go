package apple2

import (
	"encoding/hex"
	"fmt"
)

// FixedNameTable is an evidence-preserving view of a caller-selected table in
// SYSTEM.DATA.  The parser proves only the fixed record shape and printable
// name bytes; the semantic role (system, planet, or city) remains a separate
// evidence field.
type FixedNameTable struct {
	Name             string           `json:"name"`
	RawOffset        int              `json:"rawOffset"`
	RecordSize       int              `json:"recordSize"`
	RecordCount      int              `json:"recordCount"`
	Entries          []FixedNameEntry `json:"entries"`
	ObservationLevel EvidenceLevel    `json:"observationEvidence"`
	SemanticLabel    string           `json:"semanticLabel,omitempty"`
	SemanticLevel    EvidenceLevel    `json:"semanticEvidence"`
	ConsumerLevel    EvidenceLevel    `json:"consumerEvidence"`
}

// FixedNameEntry keeps the full record and the bytes after the name.  Those
// tail bytes are intentionally opaque: they may contain coordinates, flags,
// indices, or unrelated packed fields until a live consumer is traced.
type FixedNameEntry struct {
	Index     int    `json:"index"`
	RawOffset int    `json:"rawOffset"`
	Name      string `json:"name"`
	RawHex    string `json:"rawHex"`
	TailHex   string `json:"tailHex"`
}

// ParseFixedNameTable parses a caller-selected fixed-record candidate from a
// raw Apple II span.  It does not scan for tables or infer record size.  The
// caller must provide every structural boundary so a plausible byte pattern
// cannot silently become a filesystem or game-data claim.
func ParseFixedNameTable(data []byte, name string, rawOffset, recordSize, recordCount int) (FixedNameTable, error) {
	if name == "" {
		return FixedNameTable{}, fmt.Errorf("table name is empty")
	}
	if rawOffset < 0 || recordSize < 2 || recordCount <= 0 {
		return FixedNameTable{}, fmt.Errorf("invalid table bounds: offset=%d recordSize=%d recordCount=%d", rawOffset, recordSize, recordCount)
	}
	end := rawOffset + recordSize*recordCount
	if end < rawOffset || end > len(data) {
		return FixedNameTable{}, fmt.Errorf("table %q ends outside image: 0x%x", name, end)
	}
	entries := make([]FixedNameEntry, 0, recordCount)
	for index := 0; index < recordCount; index++ {
		offset := rawOffset + index*recordSize
		record := data[offset : offset+recordSize]
		nameLength := int(record[0])
		if nameLength <= 0 || nameLength >= recordSize {
			return FixedNameTable{}, fmt.Errorf("table %q record %d at 0x%x has invalid name length %d", name, index, offset, nameLength)
		}
		nameBytes := record[1 : 1+nameLength]
		for _, value := range nameBytes {
			if value < 0x20 || value > 0x7e {
				return FixedNameTable{}, fmt.Errorf("table %q record %d at 0x%x has non-printable name byte 0x%02x", name, index, offset, value)
			}
		}
		entries = append(entries, FixedNameEntry{
			Index:     index,
			RawOffset: offset,
			Name:      string(nameBytes),
			RawHex:    hex.EncodeToString(record),
			TailHex:   hex.EncodeToString(record[1+nameLength:]),
		})
	}
	return FixedNameTable{
		Name:             name,
		RawOffset:        rawOffset,
		RecordSize:       recordSize,
		RecordCount:      recordCount,
		Entries:          entries,
		ObservationLevel: EvidenceProven,
		SemanticLevel:    EvidenceUnknown,
		ConsumerLevel:    EvidenceUnknown,
	}, nil
}
