package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"godump/config"

	"gopkg.in/yaml.v3"
)

const (
	sourceManaged = "managed"
	sourceConfig  = "config"
	maxManaged    = 50
	prefixLen     = 11 // "gd_" plus 8 hex characters
)

var (
	ErrNotFound      = errors.New("API key not found")
	ErrConfigManaged = errors.New("This key is defined in the configuration file. Remove it there and restart GoDump.")
	ErrNameRequired  = errors.New("Name is required")
	ErrNameTooLong   = errors.New("Name must be 80 characters or fewer")
	ErrTooMany       = errors.New("Too many API keys")
	errInvalidHash   = errors.New("invalid API key hash")
)

// KeyInfo is a stored key without the secret.
type KeyInfo struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Prefix    string  `json:"prefix"`
	CreatedAt *string `json:"created_at"`
	Source    string  `json:"source"`
}

// CreatedKey is returned once, when a key is created.
type CreatedKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	CreatedAt string `json:"created_at"`
	Source    string `json:"source"`
	Key       string `json:"key"`
}

type record struct {
	id        string
	name      string
	prefix    string
	hash      string
	hashBytes []byte
	createdAt time.Time
	source    string
}

type storedFile struct {
	Keys []storedKey `yaml:"keys"`
}

type storedKey struct {
	ID        string    `yaml:"id"`
	Name      string    `yaml:"name"`
	Prefix    string    `yaml:"prefix"`
	Hash      string    `yaml:"hash"`
	CreatedAt time.Time `yaml:"created_at"`
}

// Store holds hashed API keys from the configuration file and from the UI.
type Store struct {
	mu   sync.RWMutex
	path string
	keys []record
}

// Open loads keys created in the UI plus keys declared in the configuration.
// UI keys are stored as hashes in a file beside the configuration, unless
// api_keys_file is set.
func Open(configPath string, cfg *config.Config) (*Store, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	path := cfg.APIKeysFile
	if path == "" {
		path = filepath.Join(filepath.Dir(configPath), "api-keys.yaml")
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(configPath), path)
	}

	s := &Store{path: path}
	if err := s.loadFile(); err != nil {
		return nil, err
	}
	if err := s.addConfigKeys(cfg.APIKeys); err != nil {
		return nil, err
	}
	return s, nil
}

// Path returns the file used for keys created in the UI.
func (s *Store) Path() string {
	return s.path
}

// List returns every key, without hashes or secrets.
func (s *Store) List() []KeyInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]KeyInfo, 0, len(s.keys))
	for _, key := range s.keys {
		out = append(out, key.info())
	}
	return out
}

// Create generates a key, stores only its hash, and returns the full key once.
func (s *Store) Create(name string) (CreatedKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CreatedKey{}, ErrNameRequired
	}
	if len(name) > 80 {
		return CreatedKey{}, ErrNameTooLong
	}

	token, err := newToken()
	if err != nil {
		return CreatedKey{}, err
	}
	id, err := newID()
	if err != nil {
		return CreatedKey{}, err
	}

	sum := sha256.Sum256([]byte(token))
	rec := record{
		id:        id,
		name:      name,
		prefix:    token[:prefixLen],
		hash:      hex.EncodeToString(sum[:]),
		hashBytes: append([]byte(nil), sum[:]...),
		createdAt: time.Now().UTC().Truncate(time.Second),
		source:    sourceManaged,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.managedCount() >= maxManaged {
		return CreatedKey{}, ErrTooMany
	}
	s.keys = append(s.keys, rec)
	if err := s.saveLocked(); err != nil {
		s.keys = s.keys[:len(s.keys)-1]
		return CreatedKey{}, err
	}

	created := rec.createdAt.Format(time.RFC3339)
	return CreatedKey{
		ID:        rec.id,
		Name:      rec.name,
		Prefix:    rec.prefix,
		CreatedAt: created,
		Source:    rec.source,
		Key:       token,
	}, nil
}

// Revoke removes a key created in the UI. Configuration keys stay until removed from the file.
func (s *Store) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, key := range s.keys {
		if key.id == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNotFound
	}
	if s.keys[idx].source == sourceConfig {
		return ErrConfigManaged
	}

	kept := append([]record(nil), s.keys[:idx]...)
	kept = append(kept, s.keys[idx+1:]...)
	previous := s.keys
	s.keys = kept
	if err := s.saveLocked(); err != nil {
		s.keys = previous
		return err
	}
	return nil
}

// Valid reports whether the token matches a stored key.
func (s *Store) Valid(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	s.mu.RLock()
	defer s.mu.RUnlock()

	match := 0
	for _, key := range s.keys {
		match |= subtle.ConstantTimeCompare(sum[:], key.hashBytes)
	}
	return match == 1
}

func (s *Store) loadFile() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read API keys: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var file storedFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse API keys: %w", err)
	}

	for _, key := range file.Keys {
		rec, err := recordFromStored(key)
		if err != nil {
			return fmt.Errorf("API key %q: %w", key.Name, err)
		}
		s.keys = append(s.keys, rec)
	}
	return nil
}

func (s *Store) addConfigKeys(keys []config.APIKeyConfig) error {
	for i, key := range keys {
		name := strings.TrimSpace(key.Name)
		if name == "" {
			return fmt.Errorf("api_keys[%d]: name is required", i)
		}
		hash := strings.ToLower(strings.TrimSpace(key.Hash))
		raw, err := decodeHash(hash)
		if err != nil {
			return fmt.Errorf("api_keys[%d] (%s): %w", i, name, err)
		}
		prefix := strings.TrimSpace(key.Prefix)
		id := configID(name, hash)
		s.keys = append(s.keys, record{
			id:        id,
			name:      name,
			prefix:    prefix,
			hash:      hash,
			hashBytes: raw,
			source:    sourceConfig,
		})
	}
	return nil
}

func (s *Store) saveLocked() error {
	file := storedFile{Keys: make([]storedKey, 0)}
	for _, key := range s.keys {
		if key.source != sourceManaged {
			continue
		}
		file.Keys = append(file.Keys, storedKey{
			ID:        key.id,
			Name:      key.name,
			Prefix:    key.prefix,
			Hash:      key.hash,
			CreatedAt: key.createdAt,
		})
	}

	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) managedCount() int {
	n := 0
	for _, key := range s.keys {
		if key.source == sourceManaged {
			n++
		}
	}
	return n
}

func (r record) info() KeyInfo {
	info := KeyInfo{
		ID:     r.id,
		Name:   r.name,
		Prefix: r.prefix,
		Source: r.source,
	}
	if !r.createdAt.IsZero() {
		created := r.createdAt.UTC().Format(time.RFC3339)
		info.CreatedAt = &created
	}
	return info
}

func recordFromStored(key storedKey) (record, error) {
	if strings.TrimSpace(key.Name) == "" {
		return record{}, errors.New("name is required")
	}
	if key.ID == "" {
		return record{}, errors.New("id is required")
	}
	hash := strings.ToLower(strings.TrimSpace(key.Hash))
	raw, err := decodeHash(hash)
	if err != nil {
		return record{}, err
	}
	return record{
		id:        key.ID,
		name:      key.Name,
		prefix:    key.Prefix,
		hash:      hash,
		hashBytes: raw,
		createdAt: key.CreatedAt,
		source:    sourceManaged,
	}, nil
}

func decodeHash(hash string) ([]byte, error) {
	raw, err := hex.DecodeString(hash)
	if err != nil || len(raw) != sha256.Size {
		return nil, errInvalidHash
	}
	return raw, nil
}

func configID(name, hash string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + hash))
	return "config-" + hex.EncodeToString(sum[:6])
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "gd_" + hex.EncodeToString(buf), nil
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
