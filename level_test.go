package golog

import "testing"

func TestLevel(t *testing.T) {
	if levelMin != Level(0) {
		t.Errorf("levelMin should be zero.")
	}

	if levelCount != 5 {
		t.Errorf("levelCount should be five.")
	}

	if levelMax != Level(4) {
		t.Errorf("levelMax should be four.")
	}
}
