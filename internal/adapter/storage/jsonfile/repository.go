// Package jsonfile — kuzatuvlarni JSON faylda saqlovchi adapter (usecase.WatchRepository).
// Postgres/SQLite'ga o'tish uchun shu interfeysni amalga oshiruvchi yangi adapter yozish kifoya.
package jsonfile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/usecase"
)

// record — saqlash formati; domen modelini json teglaridan toza saqlaydi.
type record struct {
	ID       string  `json:"id"`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Date     string  `json:"date"`
	MaxPrice float64 `json:"maxPrice"`
	MinSeats int     `json:"minSeats"`
	CarType  string  `json:"carType"`
	ChatID   string  `json:"chatId"`
	Active   bool    `json:"active"`

	CreatedAt    time.Time `json:"createdAt"`
	LastCheck    time.Time `json:"lastCheck"`
	LastStatus   string    `json:"lastStatus"`
	LastMatches  int       `json:"lastMatches"`
	LastSig      string    `json:"lastSig"`
	LastNotified time.Time `json:"lastNotified"`
}

func fromDomain(w domain.Watch) record {
	return record{
		ID: w.ID, From: w.Route.From, To: w.Route.To, Date: w.Route.Date,
		MaxPrice: w.Filter.MaxPrice, MinSeats: w.Filter.MinSeats, CarType: w.Filter.CarType,
		ChatID: w.ChatID, Active: w.Active, CreatedAt: w.CreatedAt,
		LastCheck: w.LastCheck, LastStatus: w.LastStatus, LastMatches: w.LastMatches,
		LastSig: w.LastSig, LastNotified: w.LastNotified,
	}
}

func (r record) toDomain() domain.Watch {
	return domain.Watch{
		ID:     r.ID,
		Route:  domain.Route{From: r.From, To: r.To, Date: r.Date},
		Filter: domain.Filter{MaxPrice: r.MaxPrice, MinSeats: r.MinSeats, CarType: r.CarType},
		ChatID: r.ChatID, Active: r.Active, CreatedAt: r.CreatedAt,
		LastCheck: r.LastCheck, LastStatus: r.LastStatus, LastMatches: r.LastMatches,
		LastSig: r.LastSig, LastNotified: r.LastNotified,
	}
}

type Repository struct {
	mu   sync.Mutex
	path string
	recs []record
}

var _ usecase.WatchRepository = (*Repository)(nil)

func Open(path string) (*Repository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r := &Repository{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &r.recs); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Repository) Create(_ context.Context, w domain.Watch) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w.ID = newID()
	r.recs = append(r.recs, fromDomain(w))
	return w, r.flush()
}

func (r *Repository) Get(_ context.Context, id string) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i := r.index(id); i >= 0 {
		return r.recs[i].toDomain(), nil
	}
	return domain.Watch{}, domain.ErrNotFound
}

func (r *Repository) List(_ context.Context) ([]domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Watch, 0, len(r.recs))
	for _, rec := range r.recs {
		out = append(out, rec.toDomain())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *Repository) Update(_ context.Context, id string, fn func(*domain.Watch)) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.index(id)
	if i < 0 {
		return domain.Watch{}, domain.ErrNotFound
	}
	w := r.recs[i].toDomain()
	fn(&w)
	w.ID = id
	r.recs[i] = fromDomain(w)
	return w, r.flush()
}

func (r *Repository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.index(id)
	if i < 0 {
		return domain.ErrNotFound
	}
	r.recs = append(r.recs[:i], r.recs[i+1:]...)
	return r.flush()
}

func (r *Repository) index(id string) int {
	for i, rec := range r.recs {
		if rec.ID == id {
			return i
		}
	}
	return -1
}

// flush faylni atomar yozadi (tmp + rename).
func (r *Repository) flush() error {
	b, err := json.MarshalIndent(r.recs, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
