package game

import "testing"

func statusState(t *testing.T) (Reducer, State) {
	t.Helper()
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenCharacterStatus
	return reducer, state
}

func TestTheHandTakesOneCarriedObjectAndPutsItBack(t *testing.T) {
	reducer, state := statusState(t)
	state.Parts = []int{9, 11}

	held, err := reducer.Step(state, ActionHand)
	if err != nil {
		t.Fatal(err)
	}
	if held.HandItem != 9 {
		t.Fatalf("hand holds %d, want the first carried object", held.HandItem)
	}
	if len(held.Parts) != 4 || held.Parts[0] != 0 || held.Parts[1] != 11 {
		t.Fatalf("carried = %v, want fixed slots [0 11 0 0]", held.Parts)
	}

	stowed, err := reducer.Step(held, ActionHand)
	if err != nil {
		t.Fatal(err)
	}
	if stowed.HandItem != 0 {
		t.Fatalf("hand still holds %d", stowed.HandItem)
	}
	if len(stowed.Parts) != 4 || stowed.Parts[0] != 9 || stowed.Parts[1] != 11 {
		t.Fatalf("carried = %v, want both back", stowed.Parts)
	}
}

func TestAnEmptyHandWithNothingCarriedSaysSo(t *testing.T) {
	reducer, state := statusState(t)
	state.Parts = nil
	next, err := reducer.Step(state, ActionHand)
	if err != nil {
		t.Fatal(err)
	}
	if next.HandItem != 0 || next.MessageKey != "ui.message.nothing_to_hold" {
		t.Fatalf("hand=%d message=%q", next.HandItem, next.MessageKey)
	}
}

func TestTheBodyCannotTakeMoreThanItsSlots(t *testing.T) {
	reducer, state := statusState(t)
	state.HandItem = 9
	state.Parts = []int{9, 11, 12, 13}
	next, err := reducer.Step(state, ActionHand)
	if err != nil {
		t.Fatal(err)
	}
	if next.HandItem != 9 {
		t.Fatal("a full body still took the held object")
	}
	if next.MessageKey != "ui.message.carry_full" {
		t.Fatalf("message = %q", next.MessageKey)
	}
}

func TestTheHandIsOnlyReachableInCharacterStatus(t *testing.T) {
	reducer, state := statusState(t)
	state.Screen = ScreenGround
	if _, err := reducer.Step(state, ActionHand); err == nil {
		t.Fatal("the hand was reachable from the ground screen")
	}
}

func TestWalkingTurnsTheCompass(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenGround
	if state.Heading != HeadingNorth {
		t.Fatalf("a new game starts facing %q", state.Heading)
	}
	// Whichever way the city map lets the character walk, the compass has to
	// follow that move rather than keep its old value.
	for action, want := range map[Action]Heading{
		ActionMoveNorth: HeadingNorth,
		ActionMoveSouth: HeadingSouth,
		ActionMoveWest:  HeadingWest,
		ActionMoveEast:  HeadingEast,
	} {
		next, err := reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
		if next.Position == state.Position {
			continue // blocked; the compass only turns on a move that happens
		}
		if next.Heading != want {
			t.Fatalf("%s left the compass at %q, want %q", action, next.Heading, want)
		}
	}
}
