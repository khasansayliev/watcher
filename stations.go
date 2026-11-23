package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
)

//go:embed stations.json
var defaultStations []byte

type Station struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type Stations struct {
	List   []Station
	byCode map[string]string
}

func loadStations(path string) (*Stations, error) {
	data := defaultStations
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		data = b
	}
	var list []Station
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("stations.json: %w", err)
	}
	s := &Stations{List: list, byCode: map[string]string{}}
	for _, st := range list {
		s.byCode[st.Code] = st.Name
	}
	return s, nil
}

func (s *Stations) Name(code string) string {
	if n, ok := s.byCode[code]; ok {
		return n
	}
	return code
}
