package production

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var profileIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

type ICCProfileMeta struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	FileName    string    `json:"fileName"`
	SHA256      string    `json:"sha256"`
	Size        int       `json:"size"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
	Description string    `json:"description,omitempty"`
}

type ICCProfileStore struct {
	dir string
	mu  sync.Mutex
}

func NewICCProfileStore(dir string) (*ICCProfileStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("ICC profile directory is not configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &ICCProfileStore{dir: dir}, nil
}

func (s *ICCProfileStore) List() ([]ICCProfileMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]ICCProfileMeta, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".meta.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		var meta ICCProfileMeta
		if json.Unmarshal(raw, &meta) == nil {
			out = append(out, meta)
		}
	}
	return out, nil
}

func (s *ICCProfileStore) Get(id string) (ICCProfileMeta, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.readMeta(id)
	if err != nil {
		return ICCProfileMeta{}, "", err
	}
	path := filepath.Join(s.dir, meta.FileName)
	if _, err := os.Stat(path); err != nil {
		return ICCProfileMeta{}, "", fmt.Errorf("ICC profile file missing for %s", id)
	}
	return meta, path, nil
}

func (s *ICCProfileStore) Put(id, label, description string, data []byte) (ICCProfileMeta, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if !profileIDPattern.MatchString(id) {
		return ICCProfileMeta{}, errors.New("profile id must be 2-64 chars of [a-z0-9_-]")
	}
	if len(data) < 128 || len(data) > 8<<20 {
		return ICCProfileMeta{}, errors.New("ICC profile must be between 128 bytes and 8 MB")
	}
	if !looksLikeICC(data) {
		return ICCProfileMeta{}, errors.New("file does not look like an ICC profile")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	version := 1
	if existing, err := s.readMeta(id); err == nil {
		version = existing.Version + 1
	}
	fileName := id + ".icc"
	digest := sha256.Sum256(data)
	meta := ICCProfileMeta{
		ID: id, Label: strings.TrimSpace(label), FileName: fileName, SHA256: hex.EncodeToString(digest[:]),
		Size: len(data), Version: version, CreatedAt: time.Now().UTC(), Description: strings.TrimSpace(description),
	}
	if meta.Label == "" {
		meta.Label = id
	}
	if err := os.WriteFile(filepath.Join(s.dir, fileName), data, 0o644); err != nil {
		return ICCProfileMeta{}, err
	}
	raw, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(s.dir, id+".meta.json"), raw, 0o644); err != nil {
		return ICCProfileMeta{}, err
	}
	return meta, nil
}

func (s *ICCProfileStore) readMeta(id string) (ICCProfileMeta, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, id+".meta.json"))
	if err != nil {
		return ICCProfileMeta{}, fmt.Errorf("unknown ICC profile %q", id)
	}
	var meta ICCProfileMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ICCProfileMeta{}, err
	}
	return meta, nil
}

func looksLikeICC(data []byte) bool {
	if len(data) < 128 {
		return false
	}
	// Bytes 36-39 are the profile signature 'acsp'
	return string(data[36:40]) == "acsp"
}
