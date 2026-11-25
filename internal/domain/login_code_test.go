package domain

import "testing"

func TestNewLoginCode(t *testing.T) {
	seen := map[string]bool{}
	for range 500 {
		c, err := NewLoginCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != LoginCodeLen {
			t.Fatalf("uzunlik: %q", c)
		}
		if got, ok := NormalizeLoginCode(c); !ok || got != c {
			t.Fatalf("o'zi yaratgan kod normallashmadi: %q -> %q %v", c, got, ok)
		}
		seen[c] = true
	}
	if len(seen) < 499 {
		t.Fatalf("kodlar takrorlanyapti: %d xil", len(seen))
	}
}

func TestNormalizeLoginCode(t *testing.T) {
	ok := map[string]string{
		"ABCD-EFGH":   "ABCDEFGH",
		"abcd-efgh":   "ABCDEFGH",
		" abcd efgh ": "ABCDEFGH",
		"ABCDEFGH":    "ABCDEFGH",
		"ab2d-ef34":   "AB2DEF34",
	}
	for in, want := range ok {
		if got, good := NormalizeLoginCode(in); !good || got != want {
			t.Errorf("%q -> %q, %v; kutilgan %q", in, got, good, want)
		}
	}
	for _, in := range []string{"", "ABCD", "ABCD-EFGHJ", "ABCD-EFG0", "ABCD-EFGI", "ABCD-EFGO", "ABCD-EFG1", "ABCD-EF!H", "ABCD-EFGH; DROP"} {
		if got, good := NormalizeLoginCode(in); good {
			t.Errorf("%q yaroqsiz bo'lishi kerak edi, lekin %q qaytdi", in, got)
		}
	}
	if FormatLoginCode("ABCDEFGH") != "ABCD-EFGH" {
		t.Fatal("format")
	}
}
