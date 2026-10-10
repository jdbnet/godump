package backup

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// BackupJobStatus is one backup target in the shared status contract.
// A job is one discovered database on a configured MariaDB instance.
// An instance with no discovered databases is reported as a single job so a
// connection failure is still visible. The id is "<instance>/<database>", or
// the instance name when no database has been discovered.
type BackupJobStatus struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Target              string  `json:"target"`
	Enabled             bool    `json:"enabled"`
	LastRunAt           *string `json:"last_run_at"`
	LastStatus          string  `json:"last_status"`
	LastSuccessAt       *string `json:"last_success_at"`
	LastDurationSeconds int64   `json:"last_duration_seconds"`
	LastSizeBytes       int64   `json:"last_size_bytes"`
	LastError           *string `json:"last_error"`
	NextRunAt           *string `json:"next_run_at"`
	Stale               bool    `json:"stale"`
}

// BackupStatus is the GET /api/v1/backups/status payload.
type BackupStatus struct {
	App         string            `json:"app"`
	Version     string            `json:"version"`
	GeneratedAt string            `json:"generated_at"`
	Overall     string            `json:"overall"`
	Jobs        []BackupJobStatus `json:"jobs"`
}

// BackupStatus builds the read-only status document from in-memory job state.
func (m *Manager) BackupStatus(app, version string, now time.Time) BackupStatus {
	jobs := make([]BackupJobStatus, 0)
	if m != nil {
		m.mu.RLock()
		if m.cfg != nil {
			for _, instCfg := range m.cfg.Instances {
				inst := m.instances[instCfg.Name]
				if inst == nil {
					continue
				}
				jobs = append(jobs, m.jobsForInstance(inst, now)...)
			}
		}
		m.mu.RUnlock()
	}
	return BackupStatus{
		App:         app,
		Version:     version,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Overall:     overallStatus(jobs),
		Jobs:        jobs,
	}
}

func (m *Manager) jobsForInstance(inst *InstanceStatus, now time.Time) []BackupJobStatus {
	inst.mu.RLock()
	next := inst.NextRunTime
	entryID := inst.CronEntryID
	inst.mu.RUnlock()

	if m.cron != nil && entryID != 0 {
		if cronNext := m.cron.Entry(entryID).Next; !cronNext.IsZero() {
			next = cronNext
		}
	}

	inst.mu.RLock()
	defer inst.mu.RUnlock()

	if len(inst.Databases) == 0 {
		return []BackupJobStatus{jobFromInstance(inst, next, now)}
	}

	names := make([]string, 0, len(inst.Databases))
	for name := range inst.Databases {
		names = append(names, name)
	}
	sort.Strings(names)

	jobs := make([]BackupJobStatus, 0, len(names))
	for _, name := range names {
		jobs = append(jobs, jobFromDatabase(inst, inst.Databases[name], next, now))
	}
	return jobs
}

func jobFromInstance(inst *InstanceStatus, next, now time.Time) BackupJobStatus {
	status := "never_run"
	var lastSuccess time.Time
	if inst.IsRunning || inst.OverallResult == "running" {
		status = "running"
	} else {
		switch inst.OverallResult {
		case "success":
			status = "success"
			lastSuccess = inst.LastRunTime
		case "failed":
			status = "failed"
		case "partial":
			status = "partial"
		}
	}

	return BackupJobStatus{
		ID:            inst.Config.Name,
		Name:          inst.Config.Name,
		Target:        fmt.Sprintf("%s:%d", inst.Config.Host, inst.Config.Port),
		Enabled:       true,
		LastRunAt:     formatTimePtr(inst.LastRunTime),
		LastStatus:    status,
		LastSuccessAt: formatTimePtr(lastSuccess),
		LastError:     stringPtr(inst.DiscoveryError),
		NextRunAt:     formatTimePtr(next),
		Stale:         isStale(lastSuccess, inst.Config.Schedule, now),
	}
}

func jobFromDatabase(inst *InstanceStatus, db *DBStatus, next, now time.Time) BackupJobStatus {
	lastRun := db.LastBackupTime
	var lastSuccess time.Time
	status := "never_run"
	lastErr := db.LastError

	switch {
	case db.InProgress:
		status = "running"
		if !db.RunStartedAt.IsZero() {
			lastRun = db.RunStartedAt
		}
		if db.LastBackupResult == "success" {
			lastSuccess = db.LastBackupTime
		}
		lastErr = ""
	case inst.DiscoveryError != "" && inst.OverallResult == "failed":
		// The latest instance run failed before this database was dumped.
		status = "failed"
		if !inst.LastRunTime.IsZero() {
			lastRun = inst.LastRunTime
		}
		lastErr = inst.DiscoveryError
		if db.LastBackupResult == "success" {
			lastSuccess = db.LastBackupTime
		}
	default:
		switch db.LastBackupResult {
		case "success":
			status = "success"
			lastSuccess = db.LastBackupTime
		case "failed":
			status = "failed"
		case "partial":
			status = "partial"
		case "running":
			status = "running"
		default:
			status = "never_run"
		}
	}

	return BackupJobStatus{
		ID:                  inst.Config.Name + "/" + db.Name,
		Name:                db.Name,
		Target:              fmt.Sprintf("%s:%d/%s", inst.Config.Host, inst.Config.Port, db.Name),
		Enabled:             true,
		LastRunAt:           formatTimePtr(lastRun),
		LastStatus:          status,
		LastSuccessAt:       formatTimePtr(lastSuccess),
		LastDurationSeconds: durationSeconds(db.LastBackupDuration),
		LastSizeBytes:       db.LastBackupSize,
		LastError:           stringPtr(lastErr),
		NextRunAt:           formatTimePtr(next),
		Stale:               isStale(lastSuccess, inst.Config.Schedule, now),
	}
}

func overallStatus(jobs []BackupJobStatus) string {
	enabled := 0
	warning := false
	for _, job := range jobs {
		if !job.Enabled {
			continue
		}
		enabled++
		if job.LastStatus == "failed" {
			return "failing"
		}
		if job.Stale || job.LastStatus == "partial" || job.LastStatus == "never_run" {
			warning = true
		}
	}
	if enabled == 0 {
		return "unknown"
	}
	if warning {
		return "warning"
	}
	return "ok"
}

func isStale(lastSuccess time.Time, schedule string, now time.Time) bool {
	if lastSuccess.IsZero() {
		return false
	}
	return now.Sub(lastSuccess) > staleAfter(schedule, now)
}

func staleAfter(schedule string, now time.Time) time.Duration {
	const fallback = 48 * time.Hour
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return fallback
	}
	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		return fallback
	}
	first := sched.Next(now)
	second := sched.Next(first)
	interval := second.Sub(first)
	if interval <= 0 {
		return fallback
	}
	return interval * 2
}

func durationSeconds(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64(d.Round(time.Second) / time.Second)
}

func formatTimePtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	value := t.UTC().Format(time.RFC3339)
	return &value
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
