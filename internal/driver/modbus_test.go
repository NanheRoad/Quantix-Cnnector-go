package driver

import "testing"

func TestBytesToBitsShortInputNoPanic(t *testing.T) {
	got := bytesToBits([]byte{0x01}, 16)
	if len(got) != 16 {
		t.Fatalf("expected 16 bits, got %d", len(got))
	}
	if !got[0] {
		t.Fatalf("expected first bit true")
	}
	for i := 1; i < len(got); i++ {
		if got[i] {
			t.Fatalf("expected bit %d false for short input", i)
		}
	}
}
