package game

// The original addresses a conversation by frame; the remake's corpus is
// addressed by block and line. A frame is a slice of its block: frame 0x41
// lives in block 0x40 and its first line is the block's tenth. Opening a
// conversation "at frame N" therefore means opening block N & 0xf0 at the line
// that frame starts on.
//
// This matters wherever the remake follows the original's own dispatch --
// XDOINTER:0x26 start_interaction's kind-to-frame table, the street encounters,
// the bar booth, hailing a ship. Before this table existed those openings
// passed the frame number in as a block number, which either found no block at
// all and rendered nothing (0x41, 0x31, 0x21, 0x51) or found the block and
// opened it on the wrong line (0x80, which starts thirteen lines in).
//
// Reducer.DialogBlockForFacility is deliberately not in here: the remake's own
// street facilities are not the original's places, so those pairings name a
// block directly and start at its first line.
//
// The numbers come from docs/atari-st/dialog-rendered.json.
// TestDialogFrameOpeningsMatchTheRenderedCorpus re-derives every one of them
// from that file and fails if the two drift.

type dialogFrameOpening struct {
	Block int
	Line  int
}

var dialogFrameOpenings = map[int]dialogFrameOpening{
	0x10: {0x10, 0},  // bar counter: "What can I serve you, sir"
	0x20: {0x20, 0},  // haggling over an item
	0x21: {0x20, 12}, // street kind 22: "can I interest you in a deal"
	0x31: {0x30, 3},  // street kind 18: "Can you spare N for an old man"
	0x40: {0x40, 0},  // a ship's demand: "Give us your cargo, or we'll blast you!"
	0x41: {0x40, 9},  // street kind 16: "Give me your cash or die!"
	0x50: {0x50, 0},  // equipment shop counter
	0x51: {0x50, 0},  // parts shop counter -- a chain frame into the same lines
	0x60: {0x60, 0},  // hotel desk: "Welcome sir. looking for a room"
	0x80: {0x80, 13}, // bar booth: "Hello. Care to sit down"
	0xe0: {0xe0, 0},  // the one-time developer credit
}

// dialogOpeningForFrame turns one of the original's frame numbers into the
// block and line the corpus holds it at.
func dialogOpeningForFrame(frame int) (dialogFrameOpening, bool) {
	opening, ok := dialogFrameOpenings[frame]
	return opening, ok
}
