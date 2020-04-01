package router

import "testing"

func TestPercent(t *testing.T) {
	r := percent(33, 3)
	if r != 1 {
		t.Fatalf("unexpected result of `percent(33, 3)` is %d", r)
	}
	r = percent(66, 3)
	if r != 2 {
		t.Fatalf("unexpected result of `percent(66, 3)` is  %d", r)
	}
	r = percent(99, 3)
	if r != 3 {
		t.Fatalf("unexpected result of `percent(99, 3)` is %d", r)
	}

	r = percent(100, 3)
	if r != 3 {
		t.Fatalf("unexpected result of `percent(100, 3)` is %d", r)
	}

	r = percent(1, 3)
	if r != 1 {
		t.Fatalf("unexpected result of `percent(1, 3)` is %d", r)
	}

	r = percent(0, 3)
	if r != 0 {
		t.Fatalf("unexpected result of `percent(0, 3)` is %d", r)
	}
}

func TestPercentOf(t *testing.T) {
	r := percentOf(1, 3)
	if r != 34 {
		t.Fatalf("unexpected result of `percentOf(1, 3)` is %d", r)
	}

	r = percentOf(2, 3)
	if r != 67 {
		t.Fatalf("unexpected result of `percentOf(2, 3)` is %d", r)
	}

	r = percentOf(0, 3)
	if r != 0 {
		t.Fatalf("unexpected result of `percentOf(0, 3)` is %d", r)
	}

	r = percentOf(3, 3)
	if r != 100 {
		t.Fatalf("unexpected result of `percentOf(3, 3)` is %d", r)
	}
}
