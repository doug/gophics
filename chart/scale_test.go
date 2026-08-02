package chart

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestLinearMapInvert(t *testing.T) {
	s := &Linear{Lo: 0, Hi: 100}
	if got := s.Map(50); math.Abs(float64(got)-0.5) > 1e-6 {
		t.Fatalf("Map(50) = %v, want 0.5", got)
	}
	if got := s.Invert(0.25); !approx(got, 25) {
		t.Fatalf("Invert(0.25) = %v, want 25", got)
	}
	for _, v := range []float64{0, 12.5, 33, 100} {
		if got := s.Invert(s.Map(v)); math.Abs(got-v) > 1e-4 {
			t.Fatalf("round-trip %v -> %v", v, got)
		}
	}
}

func TestLinearNiceBounds(t *testing.T) {
	s := NewLinear(0, 23) // should snap out to [0, 25]
	if lo, hi := s.Domain(); !approx(lo, 0) || !approx(hi, 25) {
		t.Fatalf("domain = [%v, %v], want [0, 25]", lo, hi)
	}
	ticks := s.Ticks(5)
	want := []float64{0, 5, 10, 15, 20, 25}
	if len(ticks) != len(want) {
		t.Fatalf("got %d ticks, want %d", len(ticks), len(want))
	}
	for i, tk := range ticks {
		if !approx(tk.Value, want[i]) {
			t.Fatalf("tick %d = %v, want %v", i, tk.Value, want[i])
		}
	}
}

func TestLinearNegativeDomain(t *testing.T) {
	s := NewLinear(-30, 80) // must contain zero and be "nice"
	lo, hi := s.Domain()
	if lo > 0 || hi < 80 || lo > -30 {
		t.Fatalf("domain [%v, %v] does not contain [-30, 80]", lo, hi)
	}
}

func TestBandScale(t *testing.T) {
	b := NewBand([]string{"a", "b", "c", "d"})
	if got := b.Map(0); math.Abs(float64(got)-0.125) > 1e-6 { // (0+0.5)/4
		t.Fatalf("Map(0) = %v, want 0.125", got)
	}
	if got := b.Map(3); math.Abs(float64(got)-0.875) > 1e-6 { // (3+0.5)/4
		t.Fatalf("Map(3) = %v, want 0.875", got)
	}
	if got := int(b.Invert(0.6)); got != 2 { // 0.6*4 = 2.4
		t.Fatalf("Invert(0.6) = %d, want 2", got)
	}
	if ticks := b.Ticks(0); len(ticks) != 4 || ticks[1].Label != "b" {
		t.Fatalf("band ticks = %+v", ticks)
	}
	if bw := b.Bandwidth(); bw <= 0 || bw > 0.25 { // (1-0.3)/4 = 0.175
		t.Fatalf("bandwidth = %v", bw)
	}
}

func TestNiceStep(t *testing.T) {
	cases := []struct {
		lo, hi float64
		target int
		want   float64
	}{
		{0, 10, 5, 2},
		{0, 100, 5, 20},
		{0, 1, 5, 0.2},
		{0, 50, 5, 10},
	}
	for _, c := range cases {
		if got := niceStep(c.lo, c.hi, c.target); !approx(got, c.want) {
			t.Fatalf("niceStep(%v,%v,%d) = %v, want %v", c.lo, c.hi, c.target, got, c.want)
		}
	}
}
