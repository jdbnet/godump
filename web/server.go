package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
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

//go:embed templates/*
var templateFS embed.FS

type Server struct {
	cfg     *config.Config
	manager *backup.Manager
	mux     *http.ServeMux
}

type FileInfo struct {
	Name      string
	Size      int64
	Timestamp time.Time
}

type DBInventory struct {
	Name  string
	Files []FileInfo
}

type InstanceInventory struct {
	Name      string
	Databases []DBInventory
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
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/icon.png", s.handleIcon)

	s.mux.HandleFunc("/", s.requireAuth(s.handleIndex))
	s.mux.HandleFunc("/api/logs", s.requireAuthAPI(s.handleLogs))
	s.mux.HandleFunc("/api/run/all", s.requireAuthAPI(s.handleRunAll))
	s.mux.HandleFunc("/api/run/", s.requireAuthAPI(s.handleRunInstance))
	s.mux.HandleFunc("/api/download", s.requireAuthAPI(s.handleDownload))
	s.mux.HandleFunc("/api/delete", s.requireAuthAPI(s.handleDelete))

	s.mux.HandleFunc("/download/", s.requireBasicAuth(s.handleLatestDownload))
}

var sessionToken string

func init() {
	b := make([]byte, 32)
	rand.Read(b)
	sessionToken = hex.EncodeToString(b)
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Auth.Enabled {
			next(w, r)
			return
		}
		cookie, err := r.Cookie("godump_session")
		if err != nil || cookie.Value != sessionToken {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireAuthAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Auth.Enabled {
			next(w, r)
			return
		}
		cookie, err := r.Cookie("godump_session")
		if err != nil || cookie.Value != sessionToken {
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

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Auth.Enabled {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if r.Method == http.MethodGet {
		tmpl, err := template.ParseFS(templateFS, "templates/login.html")
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == s.cfg.Auth.Username && password == s.cfg.Auth.Password {
			http.SetCookie(w, &http.Cookie{
				Name:     "godump_session",
				Value:    sessionToken,
				Path:     "/",
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		
		tmpl, _ := template.ParseFS(templateFS, "templates/login.html")
		tmpl.Execute(w, map[string]string{"Error": "Invalid credentials"})
		return
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "godump_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	data, err := templateFS.ReadFile("templates/icon.png")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
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

	// Path should be /download/<instance>/<db> or /download/<instance>/<db>/
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

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

type TemplateData struct {
	TotalInstances    int
	TotalDatabases    int
	UnreachableCount  int
	TotalBackupSize   int64
	AnyRunning        bool
	AuthEnabled       bool
	Instances         []backup.InstanceSnapshot
	Inventory         []InstanceInventory
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	funcMap := template.FuncMap{
		"formatBytes": formatBytes,
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "Never"
			}
			return t.Format("2006-01-02 15:04:05")
		},
	}

	tmpl, err := template.New("index.html").Funcs(funcMap).ParseFS(templateFS, "templates/index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		return
	}

	instances := s.manager.GetInstances()
	
	inventory, totalSize := s.getInventory()
	
	data := TemplateData{
		TotalInstances:  len(instances),
		Inventory:       inventory,
		TotalBackupSize: totalSize,
		AuthEnabled:     s.cfg.Auth.Enabled,
	}

	for _, inst := range instances {
		snap := inst.Snapshot()
		data.Instances = append(data.Instances, snap)
		data.TotalDatabases += len(snap.Databases)
		if snap.OverallResult == "failed" {
			data.UnreachableCount++
		}
		if snap.IsRunning {
			data.AnyRunning = true
		}
	}

	if err := tmpl.Execute(w, data); err != nil {
		logger.Error("", "Error executing template: %v", err)
	}
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
	name := strings.TrimPrefix(r.URL.Path, "/api/run/")
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
