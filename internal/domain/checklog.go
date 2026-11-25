package domain

import "time"

// CheckLog — kuzatuv uchun bitta tekshiruv so'rovining to'liq yozuvi: qachon yuborilgan,
// qancha vaqt olgan, nima qaytgan, nechta poyezd mos kelgan, xabar ketdimi.
type CheckLog struct {
	ID      int64
	WatchID int64

	ReqTime  time.Time     // so'rov yuborilgan vaqt
	Duration time.Duration // javob kelguncha o'tgan vaqt

	OK    bool   // sayt javob berdi (xato emas)
	Error string // OK=false bo'lsa sabab (masalan "tashqi API HTTP 504")

	TrainCount int  // saytdagi barcha poyezdlar
	MatchCount int  // kuzatuv shartiga mosi
	Notified   bool // Telegram xabari yuborildi
	NotifyErr  string
	Manual     bool // qo'lda ("Tekshir", yangi kuzatuv) — aks holda avtomatik sikl

	Trains []TrainSnap // saytdan kelgan natija (ixcham ko'rinishda)
}

// TrainSnap — poyezdning log uchun ixcham nusxasi.
type TrainSnap struct {
	Number    string    `json:"n"`
	Label     string    `json:"l,omitempty"`
	Departure string    `json:"d,omitempty"`
	Arrival   string    `json:"a,omitempty"`
	Matched   bool      `json:"m,omitempty"` // kuzatuv shartiga mos vagoni bor
	Cars      []CarSnap `json:"c,omitempty"`
}

type CarSnap struct {
	Label string  `json:"l"`
	Seats int     `json:"s"`
	Price float64 `json:"p,omitempty"`
}

// SnapshotTrains saytdan kelgan barcha poyezdlarni logga yozish uchun ixchamlaydi;
// shartga mos kelganlarini belgilaydi.
func SnapshotTrains(trains []Train, matches []Match) []TrainSnap {
	matched := map[string]bool{}
	for _, m := range matches {
		matched[m.Train.Number+"|"+m.Train.Departure] = true
	}
	out := make([]TrainSnap, 0, len(trains))
	for _, t := range trains {
		s := TrainSnap{
			Number: t.Number, Label: t.Label(), Departure: t.Departure, Arrival: t.Arrival,
			Matched: matched[t.Number+"|"+t.Departure],
		}
		for _, c := range t.Cars {
			s.Cars = append(s.Cars, CarSnap{Label: c.Label(), Seats: c.FreeSeats, Price: c.Price})
		}
		out = append(out, s)
	}
	return out
}
