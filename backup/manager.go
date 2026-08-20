package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"godump/config"
	"godump/logger"
	"godump/notify"

	"github.com/robfig/cron/v3"
)

type DBStatus struct {
	Name               string        `json:"name"`
	LastBackupTime     time.Time     `json:"last_backup_time"`
	LastBackupSize     int64         `json:"last_backup_size"`
	LastBackupResult   string        `json:"last_backup_result"` // success, skipped, failed
	LastBackupDuration time.Duration `json:"last_backup_duration"`
}

type InstanceStatus struct {
	Config          config.InstanceConfig
	DB              *sql.DB
	LastRunTime     time.Time
	NextRunTime     time.Time
	OverallResult   string // success, partial, failed, running
	Databases       map[string]*DBStatus
	IsRunning       bool
	CronEntryID     cron.EntryID
	mu              sync.RWMutex
}

type DBStatusSnapshot struct {
	Name               string        `json:"name"`
	LastBackupTime     time.Time     `json:"last_backup_time"`
	LastBackupSize     int64         `json:"last_backup_size"`
	LastBackupResult   string        `json:"last_backup_result"`
	LastBackupDuration time.Duration `json:"last_backup_duration"`
}

type InstanceSnapshot struct {
	Name          string             `json:"name"`
	Host          string             `json:"host"`
	LastRunTime   time.Time          `json:"last_run_time"`
	NextRunTime   time.Time          `json:"next_run_time"`
	OverallResult string             `json:"overall_result"`
	IsRunning     bool               `json:"is_running"`
	Databases     []DBStatusSnapshot `json:"databases"`
}

func (s *InstanceStatus) Snapshot() InstanceSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := InstanceSnapshot{
		Name:          s.Config.Name,
		Host:          s.Config.Host,
		LastRunTime:   s.LastRunTime,
		NextRunTime:   s.NextRunTime,
		OverallResult: s.OverallResult,
		IsRunning:     s.IsRunning,
		Databases:     make([]DBStatusSnapshot, 0, len(s.Databases)),
	}

	for _, db := range s.Databases {
		snap.Databases = append(snap.Databases, DBStatusSnapshot{
			Name:               db.Name,
			LastBackupTime:     db.LastBackupTime,
			LastBackupSize:     db.LastBackupSize,
			LastBackupResult:   db.LastBackupResult,
			LastBackupDuration: db.LastBackupDuration,
		})
	}
	
	sort.Slice(snap.Databases, func(i, j int) bool {
		return snap.Databases[i].Name < snap.Databases[j].Name
	})

	return snap
}

type Manager struct {
	cfg       *config.Config
	instances map[string]*InstanceStatus
	cron      *cron.Cron
	mu        sync.RWMutex
}

func NewManager(cfg *config.Config) *Manager {
	c := cron.New()
	c.Start()

	m := &Manager{
		cfg:       cfg,
		instances: make(map[string]*InstanceStatus),
		cron:      c,
	}

	for _, instCfg := range cfg.Instances {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true", instCfg.User, instCfg.Password, instCfg.Host, instCfg.Port)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			logger.Error(instCfg.Name, "Failed to initialize database pool: %v", err)
		}

		status := &InstanceStatus{
			Config:    instCfg,
			DB:        db,
			Databases: make(map[string]*DBStatus),
		}
		m.instances[instCfg.Name] = status

		// Schedule cron job
		if instCfg.Schedule != "" {
			var id cron.EntryID
			id, err := c.AddFunc(instCfg.Schedule, func(name string) func() {
				return func() {
					m.RunInstance(name)
				}
			}(instCfg.Name))
			
			if err != nil {
				logger.Error(instCfg.Name, "Failed to schedule cron job: %v", err)
			} else {
				status.CronEntryID = id
				status.NextRunTime = c.Entry(id).Next
				logger.Info(instCfg.Name, "Scheduled backups, next run at %v", status.NextRunTime)
			}
		}
	}

	return m
}

func (m *Manager) GetInstances() []*InstanceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*InstanceStatus
	for _, instCfg := range m.cfg.Instances {
		if status, exists := m.instances[instCfg.Name]; exists {
			result = append(result, status)
		}
	}
	return result
}

func (m *Manager) GetInstance(name string) *InstanceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instances[name]
}

func (m *Manager) DiscoverInitial() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name, inst := range m.instances {
		dbs, err := discoverDatabases(inst.DB, inst.Config)
		if err != nil {
			logger.Error(name, "Initial database discovery failed: %v", err)
			inst.mu.Lock()
			inst.OverallResult = "failed"
			inst.mu.Unlock()
			continue
		}

		inst.mu.Lock()
		var latestInstanceTime time.Time
		var hasAnyBackup bool

		for _, db := range dbs {
			if _, exists := inst.Databases[db]; !exists {
				var lastTime time.Time
				var lastSize int64
				var lastResult string

				dbPath := filepath.Join(inst.Config.BackupDir, db)
				if entries, err := os.ReadDir(dbPath); err == nil {
					for _, e := range entries {
						if e.IsDir() {
							continue
						}
						if info, err := e.Info(); err == nil {
							if info.ModTime().After(lastTime) {
								lastTime = info.ModTime()
								lastSize = info.Size()
								lastResult = "success"
							}
						}
					}
				}

				inst.Databases[db] = &DBStatus{
					Name:             db,
					LastBackupTime:   lastTime,
					LastBackupSize:   lastSize,
					LastBackupResult: lastResult,
				}
				if lastTime.After(latestInstanceTime) {
					latestInstanceTime = lastTime
				}
				if lastResult != "" {
					hasAnyBackup = true
				}
				logger.Info(name, "Discovered initial database: %s", db)
			}
		}
		
		if latestInstanceTime.After(inst.LastRunTime) {
			inst.LastRunTime = latestInstanceTime
		}
		if hasAnyBackup && inst.OverallResult == "" {
			inst.OverallResult = "success"
		}
		inst.mu.Unlock()
	}
}

func (m *Manager) RunAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name := range m.instances {
		go m.RunInstance(name)
	}
}

func (m *Manager) RunInstance(name string) {
	inst := m.GetInstance(name)
	if inst == nil {
		return
	}

	inst.mu.Lock()
	if inst.IsRunning {
		inst.mu.Unlock()
		logger.Warn(name, "Backup job already running, skipping")
		return
	}
	inst.IsRunning = true
	inst.OverallResult = "running"
	inst.mu.Unlock()

	defer func() {
		inst.mu.Lock()
		inst.IsRunning = false
		inst.LastRunTime = time.Now()
		if inst.CronEntryID != 0 {
			inst.NextRunTime = m.cron.Entry(inst.CronEntryID).Next
		}
		inst.mu.Unlock()
	}()

	logger.Info(name, "Starting backup job")

	// 1. Discovery
	dbs, err := discoverDatabases(inst.DB, inst.Config)
	if err != nil {
		logger.Error(name, "Database discovery failed: %v", err)
		inst.mu.Lock()
		inst.OverallResult = "failed"
		inst.mu.Unlock()
		return
	}
	
	sort.Strings(dbs)

	inst.mu.Lock()
	for _, db := range dbs {
		if _, exists := inst.Databases[db]; !exists {
			inst.Databases[db] = &DBStatus{
				Name:            db,
			}
			logger.Info(name, "Discovered new database: %s", db)
		}
	}
	inst.mu.Unlock()

	// 2. Backup Execution
	successCount := 0
	failedCount := 0

	// We only backup the discovered databases. If a DB disappeared, it will not be in `dbs`.
	for _, db := range dbs {
		logger.Info(name, "Starting backup for database %s", db)
		start := time.Now()
		size, err := backupDatabase(inst.Config, db)
		duration := time.Since(start)

		inst.mu.Lock()
		dbStatus := inst.Databases[db]
		dbStatus.LastBackupTime = time.Now()
		dbStatus.LastBackupDuration = duration
		if err != nil {
			logger.Error(name, "Failed backup for database %s: %v", db, err)
			dbStatus.LastBackupResult = "failed"
			failedCount++
		} else {
			logger.Info(name, "Completed backup for database %s in %v, size %d bytes", db, duration, size)
			dbStatus.LastBackupResult = "success"
			dbStatus.LastBackupSize = size
			successCount++
		}
		inst.mu.Unlock()
	}

	// 3. Retention Enforcement
	logger.Info(name, "Running retention policy cleanup (keep %d days)", inst.Config.RetentionDays)
	deleted, err := enforceRetention(inst.Config)
	if err != nil {
		logger.Error(name, "Retention cleanup encountered errors: %v", err)
	} else {
		logger.Info(name, "Retention cleanup finished, deleted %d files", deleted)
	}

	inst.mu.Lock()
	if failedCount == 0 {
		inst.OverallResult = "success"
	} else if successCount == 0 {
		inst.OverallResult = "failed"
	} else {
		inst.OverallResult = "partial"
	}
	
	payload := notify.Payload{
		InstanceName:  name,
		OverallResult: inst.OverallResult,
		Time:          time.Now(),
	}
	var totalDuration time.Duration
	for _, db := range dbs {
		dbStat := inst.Databases[db]
		payload.Databases = append(payload.Databases, notify.DBResult{
			Name:     dbStat.Name,
			Size:     dbStat.LastBackupSize,
			Result:   dbStat.LastBackupResult,
			Duration: dbStat.LastBackupDuration,
		})
		totalDuration += dbStat.LastBackupDuration
	}
	payload.TotalDuration = totalDuration
	inst.mu.Unlock()
	
	logger.Info(name, "Backup job completed. Result: %s", inst.OverallResult)
	notify.Send(m.cfg.Notifications, payload)
}
