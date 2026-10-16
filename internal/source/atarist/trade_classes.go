package atarist

import (
	"encoding/json"
	"fmt"
	"io"
)

// The trade class table is logical block 7 of the game disk: eighteen records,
// one a planet, each carrying a supply class and a demand class for goods 1–30
// plus the planet's own produce. XDOTRADI:0x07 get_data reads it every time the
// exchange opens; init_sd seeds a city's tables from it on the first visit and
// adjust ages them back up to it afterwards. See
// docs/atari-st/trade-classes.json and tools/atari_st_trade_classes.py.

const (
	// TradeClassRecords is one record a planet.
	TradeClassRecords = 18
	// TradeClassGoods is how many goods a record covers: 6 words of five
	// three-bit fields.
	TradeClassGoods = 30
	// TradeClassMax is the largest a class can be, three bits.
	TradeClassMax = 7
)

// TradeClassRecord is one planet's row of the table.
type TradeClassRecord struct {
	PlanetIndex int
	// Supply and Demand are indexed by goods − 1.
	Supply []int
	Demand []int
	// HomeGoods is the produce get_data gives supply 7 and demand 0. Zero
	// means the planet has none.
	HomeGoods int
	// DemandGoods is word13: supply0/demand7, after the word6 override.
	DemandGoods int
}

// TradeClassData is the validated table.
type TradeClassData struct {
	records        []TradeClassRecord
	startupCounter int
}

type tradeClassDocument struct {
	Schema int `json:"schema"`
	Source struct {
		StartupCounter *int `json:"startupCounter"`
	} `json:"source"`
	Layout struct {
		RecordCount int `json:"recordCount"`
		GoodsCount  int `json:"goodsCount"`
	} `json:"layout"`
	Records []struct {
		PlanetIndex     int   `json:"planetIndex"`
		Supply          []int `json:"supply"`
		Demand          []int `json:"demand"`
		SupplyHomeGoods *int  `json:"supplyHomeGoods"`
		DemandHomeGoods *int  `json:"demandHomeGoods"`
	} `json:"records"`
}

// LoadTradeClassData validates the table. The checks are the ones a wrong
// layout would fail: eighteen records in order, thirty goods each, and every
// class inside three bits.
func LoadTradeClassData(reader io.Reader) (TradeClassData, error) {
	var document tradeClassDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return TradeClassData{}, fmt.Errorf("decode Atari ST trade classes: %w", err)
	}
	if document.Schema != 1 {
		return TradeClassData{}, fmt.Errorf("unsupported Atari ST trade class schema %d", document.Schema)
	}
	if document.Source.StartupCounter == nil || *document.Source.StartupCounter < 0 || *document.Source.StartupCounter > 255 {
		return TradeClassData{}, fmt.Errorf("missing or invalid trade startup counter")
	}
	if document.Layout.RecordCount != TradeClassRecords || document.Layout.GoodsCount != TradeClassGoods {
		return TradeClassData{}, fmt.Errorf("trade classes declare %d × %d, want %d × %d",
			document.Layout.RecordCount, document.Layout.GoodsCount, TradeClassRecords, TradeClassGoods)
	}
	if len(document.Records) != TradeClassRecords {
		return TradeClassData{}, fmt.Errorf("trade classes hold %d records, want %d",
			len(document.Records), TradeClassRecords)
	}
	records := make([]TradeClassRecord, 0, TradeClassRecords)
	for position, entry := range document.Records {
		if entry.PlanetIndex != position {
			return TradeClassData{}, fmt.Errorf("trade class record %d declares planet %d", position, entry.PlanetIndex)
		}
		if len(entry.Supply) != TradeClassGoods || len(entry.Demand) != TradeClassGoods {
			return TradeClassData{}, fmt.Errorf("trade class record %d covers %d/%d goods, want %d",
				position, len(entry.Supply), len(entry.Demand), TradeClassGoods)
		}
		for goods := 0; goods < TradeClassGoods; goods++ {
			for _, value := range []int{entry.Supply[goods], entry.Demand[goods]} {
				if value < 0 || value > TradeClassMax {
					return TradeClassData{}, fmt.Errorf("trade class record %d goods %d is %d, not three bits",
						position, goods+1, value)
				}
			}
		}
		if entry.SupplyHomeGoods == nil || *entry.SupplyHomeGoods < 0 || *entry.SupplyHomeGoods > TradeClassGoods {
			return TradeClassData{}, fmt.Errorf("trade class record %d has missing or invalid supply goods", position)
		}
		if entry.DemandHomeGoods == nil || *entry.DemandHomeGoods < 0 || *entry.DemandHomeGoods > TradeClassGoods {
			return TradeClassData{}, fmt.Errorf("trade class record %d has missing or invalid demand goods", position)
		}
		records = append(records, TradeClassRecord{
			PlanetIndex: entry.PlanetIndex,
			Supply:      append([]int(nil), entry.Supply...),
			Demand:      append([]int(nil), entry.Demand...),
			HomeGoods:   *entry.SupplyHomeGoods,
			DemandGoods: *entry.DemandHomeGoods,
		})
	}
	return TradeClassData{records: records, startupCounter: *document.Source.StartupCounter}, nil
}

// StartupCounter is logical block7 byte0x1f8 from the source input.
func (d TradeClassData) StartupCounter() int { return d.startupCounter }

// Record is one planet's row, or false when the index is outside the table.
func (d TradeClassData) Record(planetIndex int) (TradeClassRecord, bool) {
	if planetIndex < 0 || planetIndex >= len(d.records) {
		return TradeClassRecord{}, false
	}
	return d.records[planetIndex], true
}

// Records is the whole table in planet order.
func (d TradeClassData) Records() []TradeClassRecord {
	out := make([]TradeClassRecord, len(d.records))
	copy(out, d.records)
	return out
}
