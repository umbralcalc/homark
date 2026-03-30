package spine

import (
	"testing"
	"time"
)

func TestFYStartForCalendar(t *testing.T) {
	cases := []struct {
		y, m, d, want int
	}{
		{2024, 6, 15, 2024},  // June -> FY 2024-25
		{2024, 3, 31, 2023},  // March -> FY 2023-24
		{2024, 4, 1, 2024},
	}
	for _, c := range cases {
		got := FYStartForCalendar(time.Date(c.y, time.Month(c.m), c.d, 0, 0, 0, 0, time.UTC))
		if got != c.want {
			t.Fatalf("FYStart(%d-%02d-%02d)=%d want %d", c.y, c.m, c.d, got, c.want)
		}
	}
}

func TestParseONSNumber(t *testing.T) {
	v, ok := parseONSNumber("1,270")
	if !ok || v != 1270 {
		t.Fatalf("got %v %v", v, ok)
	}
	_, ok = parseONSNumber("[x]")
	if ok {
		t.Fatal("expected false")
	}
}
