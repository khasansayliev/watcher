package domain

import "time"

// DateLayout — sanalar formati (YYYY-MM-DD).
const DateLayout = "2006-01-02"

type Station struct {
	Code string
	Name string
}

// Route — qidiruv kaliti: qayerdan, qayerga, qaysi sana.
type Route struct {
	From string
	To   string
	Date string
}

func (r Route) Key() string { return r.From + "|" + r.To + "|" + r.Date }

// Validate yo'nalish va sanani tekshiradi. today: DateLayout formatida.
func (r Route) Validate(today string) error {
	if r.From == "" || r.To == "" || r.From == r.To {
		return NewValidationError("Qayerdan va qayerga har xil bo'lishi kerak")
	}
	if _, err := time.Parse(DateLayout, r.Date); err != nil {
		return NewValidationError("Sana noto'g'ri")
	}
	if r.Date < today {
		return NewValidationError("Sana o'tib ketgan")
	}
	return nil
}

// Car — poyezddagi bitta vagon turi/tarif.
type Car struct {
	Type      string  // masalan: Купе, Плацкарт, Сидячий
	Class     string  // xizmat klassi kodi: 2E, 1В ...
	FreeSeats int     // bo'sh joylar
	Price     float64 // so'm
}

func (c Car) Label() string {
	switch {
	case c.Type != "" && c.Class != "":
		return c.Type + " (" + c.Class + ")"
	case c.Type != "":
		return c.Type
	case c.Class != "":
		return c.Class
	default:
		return "vagon"
	}
}

type Train struct {
	Number    string
	Type      string
	Brand     string
	Departure string
	Arrival   string
	Duration  string
	Cars      []Car
}

func (t Train) Label() string {
	if t.Brand != "" {
		return t.Brand
	}
	return t.Type
}
