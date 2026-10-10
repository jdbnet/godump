package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"godump/apikey"
	"godump/backup"
	"godump/config"
	"godump/logger"
)

func testServer(t *testing.T, cfg *config.Config) (*Server, *apikey.Store) {
	t.Helper()
	if err := logger.Init(""); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	keys, err := apikey.Open(filepath.Join(dir, "config.yaml"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	manager := backup.NewManager(cfg)
	t.Cleanup(manager.Stop)
	return NewServer(cfg, manager, "1.2.3", keys), keys
}

func do(handler http.Handler, method, path string, body io.Reader, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestV1RequiresAPIKey(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{Enabled: false},
	}
	server, keys := testServer(t, cfg)
	created, err := keys.Create("dashboard")
	if err != nil {
		t.Fatal(err)
	}

	missing := do(server.mux, http.MethodGet, "/api/v1/health", nil, nil)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status %d", missing.Code)
	}
	var denied map[string]string
	if err := json.Unmarshal(missing.Body.Bytes(), &denied); err != nil {
		t.Fatal(err)
	}
	if denied["error"] != "unauthorised" || !strings.Contains(missing.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("body %s type %s", missing.Body.String(), missing.Header().Get("Content-Type"))
	}

	bad := do(server.mux, http.MethodGet, "/api/v1/backups/status", nil, http.Header{
		"Authorization": []string{"Bearer gd_not-a-real-key"},
	})
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad key status %d", bad.Code)
	}

	bearer := do(server.mux, http.MethodGet, "/api/v1/health", nil, http.Header{
		"Authorization": []string{"bearer " + created.Key},
	})
	if bearer.Code != http.StatusOK {
		t.Fatalf("bearer status %d body %s", bearer.Code, bearer.Body.String())
	}
	var health map[string]string
	if err := json.Unmarshal(bearer.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["app"] != "godump" || health["version"] != "1.2.3" || health["status"] != "ok" {
		t.Fatalf("health %+v", health)
	}

	headerKey := do(server.mux, http.MethodGet, "/api/v1/backups/status", nil, http.Header{
		"X-API-Key": []string{created.Key},
	})
	if headerKey.Code != http.StatusOK {
		t.Fatalf("x-api-key status %d", headerKey.Code)
	}
	var status map[string]any
	if err := json.Unmarshal(headerKey.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["app"] != "godump" || status["overall"] != "unknown" {
		t.Fatalf("status %+v", status)
	}
	jobs, ok := status["jobs"].([]any)
	if !ok || len(jobs) != 0 {
		t.Fatalf("jobs %#v", status["jobs"])
	}

	post := do(server.mux, http.MethodPost, "/api/v1/health", nil, http.Header{
		"Authorization": []string{"Bearer " + created.Key},
	})
	if post.Code != http.StatusNotFound {
		t.Fatalf("post health %d", post.Code)
	}

	legacy := do(server.mux, http.MethodGet, "/api/status", nil, nil)
	if legacy.Code != http.StatusOK {
		t.Fatalf("legacy status with auth disabled: %d", legacy.Code)
	}
}

func TestAPIKeyCannotMutate(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{Enabled: true, Username: "admin", Password: "secret"},
	}
	server, keys := testServer(t, cfg)
	created, err := keys.Create("dashboard")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"Authorization": []string{"Bearer " + created.Key}}

	run := do(server.mux, http.MethodPost, "/api/run/all", nil, header)
	if run.Code != http.StatusUnauthorized {
		t.Fatalf("run/all %d", run.Code)
	}
	list := do(server.mux, http.MethodGet, "/api/keys", nil, header)
	if list.Code != http.StatusUnauthorized {
		t.Fatalf("list keys with api key %d", list.Code)
	}
	existing := do(server.mux, http.MethodGet, "/api/status", nil, header)
	if existing.Code != http.StatusUnauthorized {
		t.Fatalf("status with api key %d", existing.Code)
	}

	session := http.Header{"Cookie": []string{"godump_session=" + sessionToken}}
	ok := do(server.mux, http.MethodGet, "/api/status", nil, session)
	if ok.Code != http.StatusOK {
		t.Fatalf("session status %d", ok.Code)
	}
	v1 := do(server.mux, http.MethodGet, "/api/v1/health", nil, session)
	if v1.Code != http.StatusUnauthorized {
		t.Fatalf("session should not authorise v1, got %d", v1.Code)
	}
}

func TestKeyManagementRoundTrip(t *testing.T) {
	sumBytes := sha256.Sum256([]byte("gd_config"))
	sum := hex.EncodeToString(sumBytes[:])
	cfg := &config.Config{
		Auth: config.AuthConfig{Enabled: true, Username: "admin", Password: "secret"},
		APIKeys: []config.APIKeyConfig{{
			Name: "from-config",
			Hash: sum,
		}},
	}
	server, keys := testServer(t, cfg)
	session := http.Header{
		"Cookie":       []string{"godump_session=" + sessionToken},
		"Content-Type": []string{"application/json"},
	}

	createdRes := do(server.mux, http.MethodPost, "/api/keys", bytes.NewBufferString(`{"name":" Dashboard "}`), session)
	if createdRes.Code != http.StatusCreated {
		t.Fatalf("create %d %s", createdRes.Code, createdRes.Body.String())
	}
	var created apikey.CreatedKey
	if err := json.Unmarshal(createdRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "Dashboard" || created.Key == "" || !keys.Valid(created.Key) {
		t.Fatalf("created %+v", created)
	}

	data, err := os.ReadFile(keys.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), created.Key) {
		t.Fatal("stored the raw key")
	}

	listRes := do(server.mux, http.MethodGet, "/api/keys", nil, session)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list %d", listRes.Code)
	}
	if strings.Contains(listRes.Body.String(), created.Key) {
		t.Fatal("list returned the raw key")
	}

	health := do(server.mux, http.MethodGet, "/api/v1/health", nil, http.Header{
		"X-API-Key": []string{created.Key},
	})
	if health.Code != http.StatusOK {
		t.Fatalf("health %d", health.Code)
	}

	revoke := do(server.mux, http.MethodDelete, "/api/keys/"+created.ID, nil, session)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke %d %s", revoke.Code, revoke.Body.String())
	}
	if keys.Valid(created.Key) {
		t.Fatal("key still valid")
	}

	var listed struct {
		Keys []apikey.KeyInfo `json:"keys"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var configID string
	for _, key := range listed.Keys {
		if key.Source == "config" {
			configID = key.ID
		}
	}
	if configID == "" {
		t.Fatal("config key missing")
	}
	denied := do(server.mux, http.MethodDelete, "/api/keys/"+configID, nil, session)
	if denied.Code != http.StatusConflict {
		t.Fatalf("revoke config %d %s", denied.Code, denied.Body.String())
	}

	blank := do(server.mux, http.MethodPost, "/api/keys", bytes.NewBufferString(`{"name":"  "}`), session)
	if blank.Code != http.StatusBadRequest {
		t.Fatalf("blank name %d", blank.Code)
	}
}
