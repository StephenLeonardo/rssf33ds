package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Store is the findings graph: an append-only list of Run and Finding nodes.
// A finding is never edited; a newer one about the same entity points at it
// through Supersedes, so the full history of every feed is recoverable.
type Store struct {
	Runs     []Run     `json:"runs"`
	Findings []Finding `json:"findings"`
}

// Run is one execution of the verifier.
type Run struct {
	ID      string    `json:"id"`
	Stage   string    `json:"stage"`
	Started time.Time `json:"started"`
	Tool    string    `json:"tool"`
}

// Finding is a claim about one entity (a feed URL), with the source that
// grounds it.
type Finding struct {
	ID         string    `json:"id"`
	Entity     string    `json:"entity"`               // found_in: the feed URL
	Name       string    `json:"name"`                 // human label
	Role       string    `json:"role"`                 // "catalog" or "candidate"
	Status     string    `json:"status"`               // valid | stale | invalid
	Claim      string    `json:"claim"`                // one-line statement
	Source     Source    `json:"source"`               // supports / contradicts
	Supersedes string    `json:"supersedes,omitempty"` // prior finding on Entity
	RunID      string    `json:"run_id"`
	At         time.Time `json:"at"`
}

// Source is the evidence for a finding.
type Source struct {
	Kind        string     `json:"kind"` // "http+parse"
	Error       string     `json:"error,omitempty"`
	Items       int        `json:"items"`
	Newest      *time.Time `json:"newest,omitempty"`
	ContentRun  int        `json:"content_runes,omitempty"`  // readable runes of the newest article
	ContentFrom string     `json:"content_origin,omitempty"` // feed | page | excerpt | empty
}

func loadStore(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Store) save(path string) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// latest returns the head (non-superseded) finding for each entity.
func (s *Store) latest() map[string]Finding {
	superseded := make(map[string]bool, len(s.Findings))
	for _, f := range s.Findings {
		if f.Supersedes != "" {
			superseded[f.Supersedes] = true
		}
	}
	heads := make(map[string]Finding)
	for _, f := range s.Findings {
		if !superseded[f.ID] {
			heads[f.Entity] = f
		}
	}
	return heads
}
