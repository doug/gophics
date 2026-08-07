package procedural

// peak/absf are local copies of the sound package's test helpers (procedural's
// tests can't import that package's internal test file).
func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func peak(buf []float32) float32 {
	var p float32
	for _, v := range buf {
		if a := absf(v); a > p {
			p = a
		}
	}
	return p
}
