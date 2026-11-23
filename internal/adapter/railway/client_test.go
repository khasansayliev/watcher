package railway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

// Soxta javob — haqiqiy formatga yaqin taxmin.
const sampleResp = `{"data":{"directions":{"forward":{"trains":[
 {"type":"Скоростной","brand":"Afrosiyob","number":"764Ф","departureDate":"05.12.2026 07:28",
  "arrivalDate":"05.12.2026 09:38","timeOnWay":"02:10","cars":[
   {"type":"Сидячий","freeSeats":12,"tariffs":[{"tariff":270000,"classServiceType":"2E"}]},
   {"type":"Сидячий","freeSeats":"0","tariffs":[{"tariff":"520 000","classServiceType":"1В"}]}]}
]}}}}`

func TestClientSearch(t *testing.T) {
	var gotBody searchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/uz/":
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "tok123", Path: "/"})
		case r.Method == http.MethodPost && r.URL.Path == searchPath:
			if r.Header.Get("X-XSRF-TOKEN") != "tok123" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			io.WriteString(w, sampleResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Lang: "uz"})
	trains, err := c.Search(context.Background(), domain.Route{From: "2900000", To: "2900700", Date: "2026-12-05"})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody.Directions.Forward.DepStationCode != "2900000" || gotBody.Directions.Forward.Date != "2026-12-05" {
		t.Fatalf("so'rov tanasi noto'g'ri: %+v", gotBody)
	}
	if len(trains) != 1 || len(trains[0].Cars) != 2 || trains[0].Label() != "Afrosiyob" {
		t.Fatalf("natija: %+v", trains)
	}
	if c2 := trains[0].Cars[1]; c2.Price != 520000 || c2.FreeSeats != 0 {
		t.Fatalf("flexNum: %+v", c2)
	}
	if len(c.LastRaw()) == 0 {
		t.Fatal("oxirgi javob saqlanmadi")
	}
}

func TestClientMapsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "x", Path: "/"})
			return
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := New(Config{BaseURL: srv.URL, Lang: "uz"}).Search(context.Background(), domain.Route{From: "1", To: "2", Date: "2026-12-05"})
	var ue *usecase.UpstreamError
	if !errors.As(err, &ue) || ue.Status != 429 || ue.RetryAfter.Seconds() != 120 {
		t.Fatalf("UpstreamError kutilgan, olindi %v", err)
	}
}
