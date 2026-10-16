package main

// Independent exterior illustration. Image0 is the visual reference for the
// cargo hull, paired blue bridge windows, green forward display and engine
// nacelles. These authored polygons never read or trace a source bitmap.
func hdTitleFreighter(b *hdMapBuilder) {
	poly := func(c uint32, points ...[2]float32) { b.polygon(c, points...) }
	poly(0x516473, [2]float32{68, 165}, [2]float32{113, 124}, [2]float32{173, 88}, [2]float32{219, 81}, [2]float32{240, 95}, [2]float32{234, 124}, [2]float32{203, 145}, [2]float32{184, 171}, [2]float32{137, 194}, [2]float32{91, 198}, [2]float32{70, 188})
	poly(0xb8c6ce, [2]float32{108, 129}, [2]float32{172, 89}, [2]float32{216, 82}, [2]float32{229, 91}, [2]float32{228, 120}, [2]float32{162, 159}, [2]float32{120, 155})
	poly(0xe4ecef, [2]float32{111, 125}, [2]float32{173, 86}, [2]float32{214, 80}, [2]float32{227, 89}, [2]float32{164, 130}, [2]float32{126, 143})
	poly(0x8195a2, [2]float32{164, 130}, [2]float32{227, 89}, [2]float32{227, 119}, [2]float32{163, 158})
	poly(0x2d4352, [2]float32{221, 92}, [2]float32{226, 89}, [2]float32{226, 117}, [2]float32{221, 120})
	poly(0xcad5db, [2]float32{178, 124}, [2]float32{197, 112}, [2]float32{217, 116}, [2]float32{218, 137}, [2]float32{195, 151}, [2]float32{175, 145})
	poly(0x7b909e, [2]float32{195, 151}, [2]float32{218, 137}, [2]float32{218, 122}, [2]float32{196, 136})
	poly(0xdfe8ec, [2]float32{112, 140}, [2]float32{135, 125}, [2]float32{158, 133}, [2]float32{159, 164}, [2]float32{137, 179}, [2]float32{112, 165})
	poly(0x90a3ae, [2]float32{136, 154}, [2]float32{159, 139}, [2]float32{159, 164}, [2]float32{137, 179})
	// Two visible nacelles have rounded caps and separate long housings.
	poly(0x879aa6, [2]float32{68, 134}, [2]float32{88, 122}, [2]float32{106, 129}, [2]float32{99, 151}, [2]float32{77, 165}, [2]float32{64, 151})
	b.ellipse(70, 144, 9, 13, 0xdbe5e9, 0)
	b.ellipse(68, 144, 6, 9, 0x8d9faa, .8)
	b.line(0x435969, .8, [2]float32{71, 133}, [2]float32{84, 125}, [2]float32{99, 130})
	poly(0x7d929f, [2]float32{129, 161}, [2]float32{152, 148}, [2]float32{173, 154}, [2]float32{170, 170}, [2]float32{147, 183}, [2]float32{131, 177})
	poly(0xe1e9ed, [2]float32{132, 160}, [2]float32{153, 151}, [2]float32{170, 156}, [2]float32{166, 164}, [2]float32{147, 174}, [2]float32{132, 172})
	b.ellipse(132, 169, 7, 9, 0xc6d3da, 0)
	b.ellipse(131, 169, 4.5, 6, 0x77909e, .65)
	b.line(0x536e80, .7, [2]float32{145, 157}, [2]float32{145, 176})
	b.line(0x536e80, .7, [2]float32{156, 153}, [2]float32{156, 170})
	// Raised bridge and the boxlike forward cabin keep the freighter identity.
	poly(0x566f80, [2]float32{97, 137}, [2]float32{123, 117}, [2]float32{143, 110}, [2]float32{161, 117}, [2]float32{158, 140}, [2]float32{128, 159}, [2]float32{104, 156})
	poly(0xe2ecef, [2]float32{97, 134}, [2]float32{125, 114}, [2]float32{143, 108}, [2]float32{158, 114}, [2]float32{138, 129}, [2]float32{111, 147})
	poly(0xabbfc9, [2]float32{111, 147}, [2]float32{138, 129}, [2]float32{158, 114}, [2]float32{158, 138}, [2]float32{129, 158}, [2]float32{107, 156})
	poly(0x1e47b3, [2]float32{134, 115}, [2]float32{141, 112}, [2]float32{154, 114}, [2]float32{155, 123}, [2]float32{148, 134}, [2]float32{131, 132})
	poly(0x398bdd, [2]float32{135, 116}, [2]float32{141, 114}, [2]float32{149, 115}, [2]float32{138, 130}, [2]float32{133, 129})
	poly(0x1f4ba5, [2]float32{161, 115}, [2]float32{167, 111}, [2]float32{169, 115}, [2]float32{166, 127}, [2]float32{159, 131})
	b.line(0x68d8ee, .7, [2]float32{162, 116}, [2]float32{164, 114}, [2]float32{162, 124})
	poly(0x9aafb9, [2]float32{68, 167}, [2]float32{82, 140}, [2]float32{109, 125}, [2]float32{128, 132}, [2]float32{130, 151}, [2]float32{106, 176}, [2]float32{86, 195}, [2]float32{72, 185})
	poly(0xe1e9ec, [2]float32{72, 163}, [2]float32{85, 140}, [2]float32{108, 129}, [2]float32{124, 134}, [2]float32{119, 153}, [2]float32{97, 169})
	poly(0xc0d0d7, [2]float32{74, 165}, [2]float32{96, 171}, [2]float32{92, 182}, [2]float32{81, 194}, [2]float32{71, 184}, [2]float32{67, 173})
	poly(0x233d4d, [2]float32{70, 171}, [2]float32{77, 166}, [2]float32{94, 172}, [2]float32{91, 181}, [2]float32{81, 190}, [2]float32{73, 182})
	poly(0x263e4d, [2]float32{81, 144}, [2]float32{94, 140}, [2]float32{109, 144}, [2]float32{104, 151}, [2]float32{83, 154})
	poly(0x5ce775, [2]float32{84, 146}, [2]float32{93, 143}, [2]float32{103, 146}, [2]float32{101, 149}, [2]float32{85, 151})
	for i := 0; i < 6; i++ {
		x, y := float32(73+i), float32(171+i*2)
		b.line(0x9fb8c6, .6, [2]float32{x, y}, [2]float32{92 - float32(i), y + 3})
	}
	for i := 0; i < 8; i++ {
		x, y := float32(132+i*7), float32(112-i*4)
		b.line(0x526d7d, .55, [2]float32{x, y}, [2]float32{x + 21, y + 1.5})
	}
	for i := 0; i < 4; i++ {
		x, y := float32(171+i*13), float32(134-i*8)
		b.line(0x304b5e, .8, [2]float32{x, y}, [2]float32{x, y + 14}, [2]float32{x + 10, y + 8}, [2]float32{x + 10, y - 6})
	}
	b.line(0xf1f6f7, .7, [2]float32{81, 140}, [2]float32{108, 126}, [2]float32{123, 131})
	b.line(0x526d7d, .65, [2]float32{93, 158}, [2]float32{111, 159}, [2]float32{119, 151})
	b.line(0x263e4c, 1, [2]float32{105, 166}, [2]float32{105, 155}, [2]float32{109, 151}, [2]float32{112, 153}, [2]float32{112, 166})
	for y := 155; y < 165; y += 2 {
		b.line(0x9ab5c3, .55, [2]float32{106, float32(y)}, [2]float32{111, float32(y) - 1})
	}
}
