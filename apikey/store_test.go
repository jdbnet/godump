package apikey

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"godump/config"
)

func TestCreateStoresHashOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	store, err := Open(cfgPath, &config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.Create("Dashboard")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Key, "gd_") {
		t.Fatalf("key %q", created.Key)
	}
	if created.Prefix != created.Key[:prefixLen] {
		t.Fatalf("prefix %q", created.Prefix)
	}
	if !store.Valid(created.Key) {
		t.Fatal("expected key to validate")
	}
	if store.Valid(created.Key+"x") || store.Valid("") {
		t.Fatal("expected invalid keys to be rejected")
	}

	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, created.Key) {
		t.Fatal("key file contains the raw key")
	}
	sum := sha256.Sum256([]byte(created.Key))
	if !strings.Contains(text, hex.EncodeToString(sum[:])) {
		t.Fatal("key file does not contain the hash")
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}

	listed := store.List()
	if len(listed) != 1 {
		t.Fatalf("listed %d", len(listed))
	}
	if listed[0].Source != sourceManaged || listed[0].CreatedAt == nil {
		t.Fatalf("list entry %+v", listed[0])
	}
}

func TestRevokeAndConfigKeys(t *testing.T) {
	dir := t.TempDir()
	raw := "gd_examplekeyexamplekeyexamplekeyexamplekeyexamplekeyexamplekeyex"
	sum := sha256.Sum256([]byte(raw))
	cfg := &config.Config{
		APIKeys: []config.APIKeyConfig{{
			Name:   "dashboard",
			Hash:   hex.EncodeToString(sum[:]),
			Prefix: "gd_example",
		}},
	}
	store, err := Open(filepath.Join(dir, "config.yaml"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Valid(raw) {
		t.Fatal("config key should validate")
	}

	listed := store.List()
	if len(listed) != 1 || listed[0].Source != sourceConfig {
		t.Fatalf("list %+v", listed)
	}
	if err := store.Revoke(listed[0].ID); err != ErrConfigManaged {
		t.Fatalf("revoke config: %v", err)
	}
	if !store.Valid(raw) {
		t.Fatal("config key should remain valid")
	}

	created, err := store.Create("ui")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(created.ID); err != nil {
		t.Fatal(err)
	}
	if store.Valid(created.Key) {
		t.Fatal("revoked key still valid")
	}
	if err := store.Revoke(created.ID); err != ErrNotFound {
		t.Fatalf("second revoke: %v", err)
	}

	reloaded, err := Open(filepath.Join(dir, "config.yaml"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Valid(created.Key) {
		t.Fatal("revoked key returned after reload")
	}
	if !reloaded.Valid(raw) {
		t.Fatal("config key missing after reload")
	}
}

func TestRejectInvalidConfigHash(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(filepath.Join(dir, "config.yaml"), &config.Config{
		APIKeys: []config.APIKeyConfig{{Name: "bad", Hash: "abcd"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateRequiresName(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "config.yaml"), &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("  "); err != ErrNameRequired {
		t.Fatalf("got %v", err)
	}
}
