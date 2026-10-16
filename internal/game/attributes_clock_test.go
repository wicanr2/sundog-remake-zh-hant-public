package game

import "testing"

func TestAttributeRestorationAcrossMultipleHours(t *testing.T) {
	s := NewState()
	s.TimeMinutes = 59
	s.Attributes = [5]int{0, 255, 4, 6, 0}
	s.AttributeMax = [5]int{50, 0, 5, 5, 2}
	advanceTime(&s, 121)
	if s.TimeMinutes != 180 || s.Attributes != [5]int{15, 240, 5, 5, 2} {
		t.Fatalf("three hour boundaries: %v at %d", s.Attributes, s.TimeMinutes)
	}
}
