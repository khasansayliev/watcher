// Package stations — stansiyalar ma'lumotnomasi (usecase.StationDirectory), JSON fayldan.
package stations

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/usecase"
)

//go:embed stations.json
var embedded []byte

type Directory struct {
	list   []domain.Station
	byCode map[string]string
}

var _ usecase.StationDirectory = (*Directory)(nil)

// Load path bo'sh bo'lsa ichki ro'yxatni ishlatadi.
func Load(path string) (*Directory, error) {
	data := embedded
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		data = b
	}
	var raw []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("stations: %w", err)
	}
	d := &Directory{byCode: make(map[string]string, len(raw))}
	for _, s := range raw {
		d.list = append(d.list, domain.Station{Code: s.Code, Name: s.Name})
		d.byCode[s.Code] = s.Name
	}
	return d, nil
}

func (d *Directory) All() []domain.Station { return d.list }

func (d *Directory) Name(code string) string {
	if n, ok := d.byCode[code]; ok {
		return n
	}
	return code
}
