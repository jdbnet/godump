package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"godump/backup"
	"godump/config"
	"godump/logger"
)

//go:embed all:static
var staticFS embed.FS

type Server struct {
	cfg     *config.Config
	manager *backup.Manager
	mux     *http.ServeMux
}

type FileInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Timestamp time.Time `json:"timestamp"`
}

type DBInventory struct {
	Name  string     `json:"name"`
	Files []FileInfo `json:"files"`
}

type InstanceInventory struct {
	Name      string        `json:"name"`
	Databases []DBInventory `json:"databases"`
}

type StatusResponse struct {
	TotalInstances   int                      `json:"total_instances"`
	TotalDatabases   int                      `json:"total_databases"`
	UnreachableCount int                      `json:"unreachable_count"`
	TotalBackupSize  int64                    `json:"total_backup_size"`
	AnyRunning       bool                     `json:"any_running"`
	Instances        []backup.InstanceSnapshot `json:"instances"`
}

type AuthResponse struct {
	Authenticated bool `json:"authenticated"`
	AuthEnabled   bool `json:"auth_enabled"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewServer(cfg *config.Config, manager *backup.Manager) *Server {
	s := &Server{
		cfg:     cfg,
		manager: manager,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.Server.Port)
	logger.Info("", "Starting web server on %s", addr)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.requireAuthAPI(s.handleAuthLogout))
	s.mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)

	s.mux.HandleFunc("GET /api/status", s.requireAuthAPI(s.handleStatus))
	s.mux.HandleFunc("GET /api/inventory", s.requireAuthAPI(s.handleInventory))
	s.mux.HandleFunc("GET /api/logs", s.requireAuthAPI(s.handleLogs))
	s.mux.HandleFunc("POST /api/run/all", s.requireAuthAPI(s.handleRunAll))
	s.mux.HandleFunc("POST /api/run/{name}", s.requireAuthAPI(s.handleRunInstance))
	s.mux.HandleFunc("GET /api/download", s.requireAuthAPI(s.handleDownload))
	s.mux.HandleFunc("POST /api/delete", s.requireAuthAPI(s.handleDelete))
	s.mux.HandleFunc("DELETE /api/delete", s.requireAuthAPI(s.handleDelete))

	s.mux.HandleFunc("GET /download/{instance}/{db}", s.requireBasicAuth(s.handleLatestDownload))
	s.mux.HandleFunc("GET /download/{instance}/{db}/", s.requireBasicAuth(s.handleLatestDownload))

	s.mux.HandleFunc("/", s.serveSPA)
}

var sessionToken string

func init() {
	b := make([]byte, 32)
	rand.Read(b)
	sessionToken = hex.EncodeToString(b)
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	if !s.cfg.Auth.Enabled {
		return true
	}
	cookie, err := r.Cookie("godump_session")
	return err == nil && cookie.Value == sessionToken
}

func (s *Server) requireAuthAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireBasicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Auth.Enabled {
			next(w, r)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok || user != s.cfg.Auth.Username || pass != s.cfg.Auth.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := AuthResponse{
		Authenticated: s.isAuthenticated(r),
		AuthEnabled:   s.cfg.Auth.Enabled,
	}

	if s.cfg.Auth.Enabled && !resp.Authenticated {
		s.writeJSON(w, http.StatusUnauthorized, resp)
		return
	}

	if !s.cfg.Auth.Enabled {
		resp.Authenticated = true
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.cfg.Auth.Enabled {
		s.writeJSON(w, http.StatusOK, AuthResponse{Authenticated: true, AuthEnabled: false})
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username != s.cfg.Auth.Username || req.Password != s.cfg.Auth.Password {
		s.writeJSON(w, http.StatusUnauthorized, AuthResponse{Authenticated: false, AuthEnabled: true})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "godump_session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	s.writeJSON(w, http.StatusOK, AuthResponse{Authenticated: true, AuthEnabled: true})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "godump_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	s.writeJSON(w, http.StatusOK, nil)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	instances := s.manager.GetInstances()
	_, totalSize := s.getInventory()

	resp := StatusResponse{
		TotalInstances:  len(instances),
		TotalBackupSize: totalSize,
	}

	for _, inst := range instances {
		snap := inst.Snapshot()
		resp.Instances = append(resp.Instances, snap)
		resp.TotalDatabases += len(snap.Databases)
		if snap.OverallResult == "failed" {
			resp.UnreachableCount++
		}
		if snap.IsRunning {
			resp.AnyRunning = true
		}
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	inventory, _ := s.getInventory()
	s.writeJSON(w, http.StatusOK, inventory)
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/download/") {
		http.NotFound(w, r)
		return
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		http.Error(w, "Static assets unavailable", http.StatusInternalServerError)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	if _, err := sub.Open(path); err != nil {
		path = "index.html"
	}

	data, err := fs.ReadFile(sub, path)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if strings.HasSuffix(path, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(path, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(path, ".png") {
		w.Header().Set("Content-Type", "image/png")
	} else if strings.HasSuffix(path, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
	}

	w.Write(data)
}

func (s *Server) resolveFilePath(instName, dbName, fileName string) (string, error) {
	inst := s.manager.GetInstance(instName)
	if inst == nil {
		return "", fmt.Errorf("instance not found")
	}

	baseDir := filepath.Clean(inst.Config.BackupDir)
	targetPath := filepath.Join(baseDir, dbName, fileName)
	targetPath = filepath.Clean(targetPath)

	if !strings.HasPrefix(targetPath, baseDir+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file path")
	}

	if _, err := os.Stat(targetPath); err != nil {
		return "", fmt.Errorf("file not found")
	}

	return targetPath, nil
}

func (s *Server) handleLatestDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/download/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 {
		http.Error(w, "Invalid path format. Expected /download/<servername>/<databasename>/", http.StatusBadRequest)
		return
	}

	instName := parts[0]
	dbName := parts[1]

	inst := s.manager.GetInstance(instName)
	if inst == nil {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	baseDir := filepath.Clean(inst.Config.BackupDir)
	dbPath := filepath.Join(baseDir, dbName)

	files, err := os.ReadDir(dbPath)
	if err != nil {
		http.Error(w, "Database backups not found", http.StatusNotFound)
		return
	}

	var latestFile os.FileInfo
	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if latestFile == nil || info.ModTime().After(latestFile.ModTime()) {
			latestFile = info
		}
	}

	if latestFile == nil {
		http.Error(w, "No backups found for this database", http.StatusNotFound)
		return
	}

	filePath, err := s.resolveFilePath(instName, dbName, latestFile.Name())
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", latestFile.Name()))
	w.Header().Set("Content-Type", "application/x-gzip")
	http.ServeFile(w, r, filePath)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	instName := q.Get("instance")
	dbName := q.Get("db")
	fileName := q.Get("file")

	if instName == "" || dbName == "" || fileName == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	filePath, err := s.resolveFilePath(instName, dbName, fileName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	w.Header().Set("Content-Type", "application/x-gzip")
	http.ServeFile(w, r, filePath)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	instName := q.Get("instance")
	dbName := q.Get("db")
	fileName := q.Get("file")

	if instName == "" || dbName == "" || fileName == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	filePath, err := s.resolveFilePath(instName, dbName, fileName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if err := os.Remove(filePath); err != nil {
		logger.Error(instName, "Failed to manually delete backup %s: %v", filePath, err)
		http.Error(w, "Failed to delete file", http.StatusInternalServerError)
		return
	}

	logger.Info(instName, "Manually deleted backup file %s", fileName)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	data, err := logger.GetRecentLogsJSON()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func (s *Server) handleRunAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.manager.RunAll()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRunInstance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "Instance name required", http.StatusBadRequest)
		return
	}
	s.manager.RunInstance(name)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getInventory() ([]InstanceInventory, int64) {
	var inv []InstanceInventory
	instances := s.manager.GetInstances()
	var totalSize int64

	for _, inst := range instances {
		instName := inst.Config.Name
		var dbInvs []DBInventory

		entries, err := os.ReadDir(inst.Config.BackupDir)
		if err != nil {
			continue
		}

		for _, dbEntry := range entries {
			if !dbEntry.IsDir() {
				continue
			}
			dbName := dbEntry.Name()
			dbPath := filepath.Join(inst.Config.BackupDir, dbName)

			files, err := os.ReadDir(dbPath)
			if err != nil {
				continue
			}

			var fileInfos []FileInfo
			for _, fileEntry := range files {
				if fileEntry.IsDir() {
					continue
				}
				info, err := fileEntry.Info()
				if err != nil {
					continue
				}
				fileInfos = append(fileInfos, FileInfo{
					Name:      info.Name(),
					Size:      info.Size(),
					Timestamp: info.ModTime(),
				})
				totalSize += info.Size()
			}
			if len(fileInfos) > 0 {
				sort.Slice(fileInfos, func(i, j int) bool {
					return fileInfos[i].Timestamp.After(fileInfos[j].Timestamp)
				})
				dbInvs = append(dbInvs, DBInventory{Name: dbName, Files: fileInfos})
			}
		}

		if len(dbInvs) > 0 {
			sort.Slice(dbInvs, func(i, j int) bool {
				return dbInvs[i].Name < dbInvs[j].Name
			})
			inv = append(inv, InstanceInventory{Name: instName, Databases: dbInvs})
		}
	}
	return inv, totalSize
}
