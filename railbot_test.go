package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Soxta javob — haqiqiy formatga yaqin taxmin.
const sampleResp = `{
 "data": {"directions": {"forward": {"trains": [
  {"type":"Скоростной","brand":"Afrosiyob","number":"764Ф",
   "departureDate":"05.12.2026 07:28","arrivalDate":"05.12.2026 09:38","timeOnWay":"02:10",
   "cars":[
     {"type":"Сидячий","freeSeats":12,"tariffs":[{"tariff":270000,"classServiceType":"2E"}]},
     {"type":"Сидячий","freeSeats":"0","tariffs":[{"tariff":"520 000","classServiceType":"1В"}]}
   ]},
  {"type":"Пассажирский","brand":"","number":"056Ф",
   "departureDate":"05.12.2026 21:10","arrivalDate":"06.12.2026 01:40","timeOnWay":"04:30",
   "cars":[{"type":"Купе","freeSeats":3,"tariffs":[{"tariff":180000,"classServiceType":"2К"}]}]}
 ]}}}
}`

func fakeAPI(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/uz/":
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "tok123", Path: "/"})
			io.WriteString(w, "<html></html>")
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/handbook/trains/list":
			if r.Header.Get("X-XSRF-TOKEN") != "tok123" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			var body map[string]map[string]map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("body: %v", err)
			}
			if body["directions"]["forward"]["depStationCode"] != "2900000" {
				t.Errorf("noto'g'ri body: %v", body)
			}
			io.WriteString(w, sampleResp)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestSearchAndMatch(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()

	rc := NewRailwayClient(srv.URL, "", "uz", t.TempDir())
	trains, err := rc.Search(context.Background(), "2900000", "2900700", "2026-12-05")
	if err != nil {
		t.Fatal(err)
	}
	if len(trains) != 2 || len(trains[0].Cars) != 2 {
		t.Fatalf("kutilmagan natija: %+v", trains)
	}
	if trains[0].Cars[1].MinPrice != 520000 || trains[0].Cars[1].FreeSeats != 0 {
		t.Fatalf("flexNum ishlamadi: %+v", trains[0].Cars[1])
	}

	ms := MatchTrains(Watch{MaxPrice: 300000}, trains)
	if len(ms) != 2 || len(ms[0].Cars) != 1 {
		t.Fatalf("filtr: %+v", ms)
	}
	ms = MatchTrains(Watch{CarType: "afrosiyob"}, trains)
	if len(ms) != 1 || ms[0].Train.Number != "764Ф" {
		t.Fatalf("turi filtri: %+v", ms)
	}
	ms = MatchTrains(Watch{MinSeats: 5}, trains)
	if len(ms) != 1 {
		t.Fatalf("joy filtri: %+v", ms)
	}

	st, _ := loadStations("")
	w := NewWatcher(nil, rc, nil, st, minPollInterval, srv.URL+"/uz/")
	msg := w.formatMessage(Watch{From: "2900000", To: "2900700", Date: "2026-12-05"}, MatchTrains(Watch{}, trains))
	t.Log("\n" + msg)
	if !strings.Contains(msg, "270 000 so'm") || !strings.Contains(msg, "Toshkent → Samarqand") {
		t.Fatal("xabar formati")
	}
}
