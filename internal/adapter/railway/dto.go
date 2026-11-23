package railway

// API DTO'lari va ularni domen modeliga o'girish.
//
// DIQQAT: javob formati rasmiy hujjatlashtirilmagan. Maydon nomlari ochiq manbalardagi
// kodga asoslangan taxmin. Sizdagi javob boshqacha bo'lsa (UI → "Oxirgi javob"),
// faqat shu fayldagi json teglarini moslang — boshqa qatlamlarga tegish shart emas.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/khasansayliev/watcher/internal/domain"
)

type searchRequest struct {
	Directions directionsReq `json:"directions"`
}
type directionsReq struct {
	Forward forwardReq `json:"forward"`
}
type forwardReq struct {
	Date           string `json:"date"`
	DepStationCode string `json:"depStationCode"`
	ArvStationCode string `json:"arvStationCode"`
}

type tariffDTO struct {
	Tariff           flexNum `json:"tariff"`
	ClassServiceType flexStr `json:"classServiceType"`
	FreeSeats        flexNum `json:"freeSeats"`
}

type carDTO struct {
	Type      flexStr     `json:"type"`
	FreeSeats flexNum     `json:"freeSeats"`
	Tariffs   []tariffDTO `json:"tariffs"`
}

type trainDTO struct {
	Type          flexStr  `json:"type"`
	Number        flexStr  `json:"number"`
	Brand         flexStr  `json:"brand"`
	DepartureDate flexStr  `json:"departureDate"`
	ArrivalDate   flexStr  `json:"arrivalDate"`
	TimeOnWay     flexStr  `json:"timeOnWay"`
	Cars          []carDTO `json:"cars"`
}

type directionsDTO struct {
	Forward struct {
		Trains []trainDTO `json:"trains"`
	} `json:"forward"`
}

type searchResponse struct {
	Data *struct {
		Directions *directionsDTO `json:"directions"`
	} `json:"data"`
	Directions *directionsDTO `json:"directions"`
}

func decodeTrains(raw []byte) ([]domain.Train, error) {
	var r searchResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("javobni o'qib bo'lmadi: %w", err)
	}
	var d *directionsDTO
	switch {
	case r.Data != nil && r.Data.Directions != nil:
		d = r.Data.Directions
	case r.Directions != nil:
		d = r.Directions
	default:
		return nil, errors.New("javobda 'directions' topilmadi — formatni tekshiring (UI → Oxirgi javob)")
	}

	out := make([]domain.Train, 0, len(d.Forward.Trains))
	for _, t := range d.Forward.Trains {
		out = append(out, t.toDomain())
	}
	return out, nil
}

func (t trainDTO) toDomain() domain.Train {
	tr := domain.Train{
		Number:    string(t.Number),
		Type:      string(t.Type),
		Brand:     string(t.Brand),
		Departure: string(t.DepartureDate),
		Arrival:   string(t.ArrivalDate),
		Duration:  string(t.TimeOnWay),
	}
	for _, c := range t.Cars {
		if len(c.Tariffs) == 0 {
			tr.Cars = append(tr.Cars, domain.Car{Type: string(c.Type), FreeSeats: int(c.FreeSeats)})
			continue
		}
		for _, tf := range c.Tariffs {
			seats := int(tf.FreeSeats)
			if seats == 0 {
				seats = int(c.FreeSeats)
			}
			tr.Cars = append(tr.Cars, domain.Car{
				Type:      string(c.Type),
				Class:     string(tf.ClassServiceType),
				FreeSeats: seats,
				Price:     float64(tf.Tariff),
			})
		}
	}
	return tr
}

// flexStr — satr yoki son kelsa ham qabul qiladi.
type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexStr(s)
		return nil
	}
	*f = flexStr(b)
	return nil
}

// flexNum — son yoki "270 000" kabi satr kelsa ham qabul qiladi.
type flexNum float64

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	s = strings.NewReplacer(" ", "", " ", "").Replace(s)
	if s == "" || s == "null" {
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		*f = flexNum(v)
	}
	return nil // noma'lum format — 0 qoldiramiz, butun javobni buzmaymiz
}
