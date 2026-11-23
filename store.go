package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Watch struct {
	ID       string  `json:"id"`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Date     string  `json:"date"` // YYYY-MM-DD
	MaxPrice float64 `json:"maxPrice"`
	CarType  string  `json:"carType"`
	MinSeats int     `json:"minSeats"`
	ChatID   string  `json:"chatId"`
	Active   bool    `json:"active"`

	CreatedAt    time.Time `json:"createdAt"`
	LastCheck    time.Time `json:"lastCheck"`
	LastStatus   string    `json:"lastStatus"`
	LastMatches  int       `json:"lastMatches"`
	LastSig      string    `json:"lastSig"`
	LastNotified time.Time `json:"lastNotified"`
}

var ErrNotFound = errors.New("kuzatuv topilmadi")

// Store — kuzatuvlarni JSON faylda saqlaydi (tashqi DB kerak emas).
type Store struct {
	mu      sync.Mutex
	path    string
	watches []*Watch
}

func NewStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.watches); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.watches, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) List() []Watch {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Watch, 0, len(s.watches))
	for _, w := range s.watches {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) Add(w Watch) (Watch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.ID = newID()
	w.CreatedAt = time.Now()
	s.watches = append(s.watches, &w)
	return w, s.saveLocked()
}

func (s *Store) Update(id string, fn func(*Watch)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.watches {
		if w.ID == id {
			fn(w)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, w := range s.watches {
		if w.ID == id {
			s.watches = append(s.watches[:i], s.watches[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
