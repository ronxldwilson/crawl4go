package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// BrowserProfile represents a persistent browser profile for authenticated crawling.
type BrowserProfile struct {
	Name      string            `json:"name"`
	UserAgent string            `json:"user_agent,omitempty"`
	Cookies   []Cookie          `json:"cookies,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Storage   map[string]string `json:"local_storage,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Cookie represents a browser cookie for profile persistence.
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite,omitempty"`
}

// BrowserProfiler manages persistent browser profiles on disk.
type BrowserProfiler struct {
	dir string
}

// NewBrowserProfiler creates a profiler that stores profiles in the given directory.
func NewBrowserProfiler(dir string) *BrowserProfiler {
	os.MkdirAll(dir, 0755)
	return &BrowserProfiler{dir: dir}
}

// Save persists a profile to disk.
func (bp *BrowserProfiler) Save(profile *BrowserProfile) error {
	profile.UpdatedAt = time.Now()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = profile.UpdatedAt
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bp.profilePath(profile.Name), data, 0600)
}

// Load reads a profile from disk.
func (bp *BrowserProfiler) Load(name string) (*BrowserProfile, error) {
	data, err := os.ReadFile(bp.profilePath(name))
	if err != nil {
		return nil, err
	}
	var profile BrowserProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// Delete removes a profile.
func (bp *BrowserProfiler) Delete(name string) error {
	return os.Remove(bp.profilePath(name))
}

// List returns all saved profile names.
func (bp *BrowserProfiler) List() ([]string, error) {
	entries, err := os.ReadDir(bp.dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	return names, nil
}

// ApplyProfile sets cookies and localStorage on a CDP session from a profile.
func ApplyProfile(sendCmd sendCmdFunc, sessionID string, profile *BrowserProfile) {
	// Set cookies
	for _, c := range profile.Cookies {
		sendCmd("Network.setCookie", map[string]any{
			"name":     c.Name,
			"value":    c.Value,
			"domain":   c.Domain,
			"path":     c.Path,
			"httpOnly": c.HTTPOnly,
			"secure":   c.Secure,
		}, sessionID)
	}
	// Set localStorage
	for key, value := range profile.Storage {
		sendCmd("Runtime.evaluate", map[string]any{
			"expression": "localStorage.setItem('" + key + "', '" + value + "')",
		}, sessionID)
	}
	// Set extra headers
	if len(profile.Headers) > 0 {
		sendCmd("Network.setExtraHTTPHeaders", map[string]any{
			"headers": profile.Headers,
		}, sessionID)
	}
}

func (bp *BrowserProfiler) profilePath(name string) string {
	return filepath.Join(bp.dir, name+".json")
}
