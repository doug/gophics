package library

import (
	"encoding/json"
	"os"
	"sync"
)

// Settings are the reader's own preferences. They are deliberately few: every
// option is a question the app failed to answer for itself, and the ones here
// are the ones that genuinely differ between people rather than between moods.
type Settings struct {
	// The three fields below are guarded by mu and must be reached through the
	// accessors: the refresh goroutine reads them while the settings screen can
	// be writing them.
	//
	// TextScale multiplies every type size in the reader. Reading size is the
	// one setting no default can get right — it depends on eyes, on the phone,
	// and on whether you are on a train.
	TextScale float64 `json:"text_scale"`

	// PrefetchImages downloads pictures during a refresh so articles read
	// offline. Turning it off halves the data a refresh uses.
	PrefetchImages bool `json:"prefetch_images"`

	// RefreshOnOpen polls when the app comes to the foreground, so the queue is
	// current without anyone pulling anything.
	RefreshOnOpen bool `json:"refresh_on_open"`

	mu   sync.RWMutex
	path string
}

// LoadSettings reads preferences, falling back to the defaults a new install
// gets.
func LoadSettings() *Settings {
	s := &Settings{TextScale: 1, PrefetchImages: true, RefreshOnOpen: true}
	s.path = path("settings.json")
	if data, err := os.ReadFile(s.path); err == nil {
		json.Unmarshal(data, s)
	}
	if s.TextScale < 0.7 || s.TextScale > 2.2 {
		s.TextScale = 1
	}
	return s
}

// Save persists the preferences.
func (s *Settings) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(struct {
		TextScale      float64 `json:"text_scale"`
		PrefetchImages bool    `json:"prefetch_images"`
		RefreshOnOpen  bool    `json:"refresh_on_open"`
	}{s.TextScale, s.PrefetchImages, s.RefreshOnOpen}, "", "  ")
	p := s.path
	s.mu.RUnlock()
	if err != nil || p == "" {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Prefetch reports whether a refresh should download pictures.
func (s *Settings) Prefetch() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PrefetchImages
}

// SetPrefetch changes it and saves.
func (s *Settings) SetPrefetch(on bool) {
	s.mu.Lock()
	s.PrefetchImages = on
	s.mu.Unlock()
	s.Save()
}

// RefreshOnLaunch reports whether to poll when the app comes to the front.
func (s *Settings) RefreshOnLaunch() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RefreshOnOpen
}

// SetRefreshOnLaunch changes it and saves.
func (s *Settings) SetRefreshOnLaunch(on bool) {
	s.mu.Lock()
	s.RefreshOnOpen = on
	s.mu.Unlock()
	s.Save()
}

// Scale returns the text multiplier.
func (s *Settings) Scale() float32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return float32(s.TextScale)
}

// SetScale changes the reading size and saves.
func (s *Settings) SetScale(v float64) {
	s.mu.Lock()
	s.TextScale = v
	s.mu.Unlock()
	s.Save()
}
