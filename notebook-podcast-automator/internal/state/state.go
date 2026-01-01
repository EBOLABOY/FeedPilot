package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusProcessed Status = "processed"
	StatusSkipped   Status = "skipped"
)

type Entry struct {
	Key   string `json:"key"`
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`

	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`

	FirstSeen time.Time `json:"first_seen,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitempty"`

	ProcessedAt time.Time `json:"processed_at,omitempty"`
	SkippedAt   time.Time `json:"skipped_at,omitempty"`

	AudioURL string `json:"audio_url,omitempty"`
	RSSGUID  string `json:"rss_guid,omitempty"`
}

type Store struct {
	path    string
	max     int
	mu      sync.Mutex
	loaded  bool
	entries map[string]Entry
}

func Open(path string, maxEntries int) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("state path required")
	}
	if maxEntries < 0 {
		maxEntries = 0
	}
	return &Store{
		path:    path,
		max:     maxEntries,
		entries: make(map[string]Entry),
	}, nil
}

func (s *Store) loadLocked() error {
	if s.loaded {
		return nil
	}
	s.loaded = true

	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	b = []byte(strings.TrimSpace(string(b)))
	if len(b) == 0 {
		return nil
	}

	var parsed map[string]Entry
	if err := json.Unmarshal(b, &parsed); err != nil {
		return fmt.Errorf("parse state file: %w", err)
	}
	if parsed == nil {
		return nil
	}
	s.entries = parsed
	return nil
}

func (s *Store) HasHandled(key string) (bool, Entry, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, Entry{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return false, Entry{}, err
	}
	e, ok := s.entries[key]
	if !ok {
		return false, Entry{}, nil
	}
	switch e.Status {
	case StatusProcessed, StatusSkipped:
		return true, e, nil
	default:
		return false, e, nil
	}
}

func (s *Store) TouchSeen(key, url, title string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	now := time.Now()
	url = strings.TrimSpace(url)
	title = strings.TrimSpace(title)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	e := s.entries[key]
	if e.Key == "" {
		e.Key = key
		e.FirstSeen = now
	}
	if e.FirstSeen.IsZero() {
		e.FirstSeen = now
	}
	e.LastSeen = now
	if e.URL == "" && url != "" {
		e.URL = url
	}
	if e.Title == "" && title != "" {
		e.Title = title
	}
	s.entries[key] = e
	return s.saveLocked()
}

func (s *Store) MarkSkipped(key, url, title, reason string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	now := time.Now()
	url = strings.TrimSpace(url)
	title = strings.TrimSpace(title)
	reason = strings.TrimSpace(reason)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	e := s.entries[key]
	if e.Key == "" {
		e.Key = key
		e.FirstSeen = now
	}
	if e.FirstSeen.IsZero() {
		e.FirstSeen = now
	}
	e.LastSeen = now
	if e.URL == "" && url != "" {
		e.URL = url
	}
	if e.Title == "" && title != "" {
		e.Title = title
	}
	if e.Status == StatusProcessed {
		// Do not downgrade a processed entry to skipped.
		s.entries[key] = e
		return s.saveLocked()
	}
	e.Status = StatusSkipped
	e.SkippedAt = now
	if reason != "" {
		e.Reason = reason
	}
	s.entries[key] = e
	return s.saveLocked()
}

func (s *Store) MarkProcessed(key, url, title, audioURL, rssGUID string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	now := time.Now()
	url = strings.TrimSpace(url)
	title = strings.TrimSpace(title)
	audioURL = strings.TrimSpace(audioURL)
	rssGUID = strings.TrimSpace(rssGUID)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	e := s.entries[key]
	if e.Key == "" {
		e.Key = key
		e.FirstSeen = now
	}
	if e.FirstSeen.IsZero() {
		e.FirstSeen = now
	}
	e.LastSeen = now
	if e.URL == "" && url != "" {
		e.URL = url
	}
	if e.Title == "" && title != "" {
		e.Title = title
	}
	e.Status = StatusProcessed
	e.ProcessedAt = now
	if audioURL != "" {
		e.AudioURL = audioURL
	}
	if rssGUID != "" {
		e.RSSGUID = rssGUID
	}
	s.entries[key] = e
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	s.pruneLocked()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	tmpPath := s.path + ".tmp"
	bakPath := s.path + ".bak"

	_ = os.Remove(tmpPath)
	if err := os.WriteFile(tmpPath, payload, 0644); err != nil {
		return err
	}

	// Best-effort backup for rollback.
	if _, err := os.Stat(s.path); err == nil {
		_ = os.Remove(bakPath)
		_ = os.Rename(s.path, bakPath)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		// Attempt to recover the previous file.
		_ = os.Remove(tmpPath)
		if _, statErr := os.Stat(bakPath); statErr == nil {
			_ = os.Remove(s.path)
			_ = os.Rename(bakPath, s.path)
		}
		return err
	}
	return nil
}

func (s *Store) pruneLocked() int {
	if s.max <= 0 {
		return 0
	}
	if len(s.entries) <= s.max {
		return 0
	}

	type item struct {
		key string
		t   time.Time
	}

	items := make([]item, 0, len(s.entries))
	for k, e := range s.entries {
		items = append(items, item{key: k, t: entrySortTime(e)})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].t.Equal(items[j].t) {
			return items[i].key > items[j].key
		}
		return items[i].t.After(items[j].t)
	})

	keep := make(map[string]bool, s.max)
	for i := 0; i < s.max && i < len(items); i++ {
		keep[items[i].key] = true
	}

	removed := 0
	for k := range s.entries {
		if keep[k] {
			continue
		}
		delete(s.entries, k)
		removed++
	}
	return removed
}

func entrySortTime(e Entry) time.Time {
	t := e.ProcessedAt
	if e.SkippedAt.After(t) {
		t = e.SkippedAt
	}
	if e.LastSeen.After(t) {
		t = e.LastSeen
	}
	if t.IsZero() {
		t = e.FirstSeen
	}
	return t
}
