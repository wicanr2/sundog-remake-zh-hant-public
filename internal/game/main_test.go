package game

import (
	"os"
	"testing"
)

// TestMain pins the new-game seed. NewState draws its opening purse the way
// the original does — from a clock — so without a fixed seed every assertion
// about a fresh game, and every replay, would move underneath the tests.
func TestMain(m *testing.M) {
	NewGameSeed = func() int64 { return 1 }
	os.Exit(m.Run())
}
