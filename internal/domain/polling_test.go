package domain

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultPolicyBoundaries(t *testing.T) {
	p := DefaultPollingPolicy()
	cases := []struct {
		date string
		want time.Duration
	}{
		{"2026-11-23", 10 * time.Second}, // bugun
		{"2026-11-24", 10 * time.Second}, // ertaga
		{"2026-11-25", 5 * time.Minute},  // 2 kun
		{"2026-12-02", 5 * time.Minute},  // 9 kun
		{"2026-12-03", time.Hour},        // 10 kun
		{"2026-12-22", time.Hour},        // 29 kun
		{"2026-12-23", 12 * time.Hour},   // 30 kun
		{"2027-03-01", 12 * time.Hour},
	}
	for _, c := range cases {
		if got := p.IntervalFor(c.date, "2026-11-23"); got != c.want {
			t.Errorf("%s: %s, kutilgan %s", c.date, got, c.want)
		}
	}
}

func TestPolicyValidation(t *testing.T) {
	ok := []PollingRule{{MaxDaysLeft: 0, Interval: time.Hour}, {MaxDaysLeft: 3, Interval: time.Minute}}
	p, err := NewPollingPolicy(ok)
	if err != nil {
		t.Fatal(err)
	}
	if p.Rules[0].MaxDaysLeft != 3 || !p.Rules[1].IsDefault() {
		t.Fatalf("tartib noto'g'ri: %+v", p.Rules)
	}

	bad := [][]PollingRule{
		{{MaxDaysLeft: 2, Interval: time.Minute}},                                                                               // qolganlari yo'q
		{{MaxDaysLeft: 0, Interval: time.Hour}, {MaxDaysLeft: 0, Interval: time.Minute}},                                        // ikkita qolganlari
		{{MaxDaysLeft: 0, Interval: time.Hour}, {MaxDaysLeft: 2, Interval: 5 * time.Second}},                                    // 10s dan kam
		{{MaxDaysLeft: 0, Interval: time.Hour}, {MaxDaysLeft: 2, Interval: time.Minute}, {MaxDaysLeft: 2, Interval: time.Hour}}, // takror
		{{MaxDaysLeft: 0, Interval: 8 * 24 * time.Hour}},                                                                        // 7 kundan ko'p
		{{MaxDaysLeft: 0, Interval: time.Hour}, {MaxDaysLeft: -1, Interval: time.Hour}},
	}
	for i, rules := range bad {
		var ve *ValidationError
		if _, err := NewPollingPolicy(rules); !errors.As(err, &ve) {
			t.Errorf("#%d: ValidationError kutilgan, olindi %v", i, err)
		}
	}
}
