package backup

import (
	"encoding/json"
	"testing"
	"time"

	"godump/config"
)

func TestBackupStatusContract(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	successAt := now.Add(-3 * time.Hour)
	next := now.Add(30 * time.Minute)

	primary := &InstanceStatus{
		Config: config.InstanceConfig{
			Name:     "primary",
			Host:     "10.0.0.5",
			Port:     3306,
			Schedule: "0 * * * *",
		},
		NextRunTime: next,
		Databases: map[string]*DBStatus{
			"zeta": {
				Name:               "zeta",
				LastBackupTime:     successAt,
				LastBackupSize:     99,
				LastBackupResult:   "success",
				LastBackupDuration: 1500 * time.Millisecond,
			},
			"app": {
				Name:             "app",
				LastBackupResult: "failed",
				LastBackupTime:   now.Add(-time.Minute),
				LastError:        "dump failed",
				LastBackupSize:   10,
			},
		},
	}
	empty := &InstanceStatus{
		Config: config.InstanceConfig{
			Name: "empty",
			Host: "10.0.0.6",
			Port: 3307,
		},
		Databases: map[string]*DBStatus{},
	}

	m := &Manager{
		cfg: &config.Config{Instances: []config.InstanceConfig{primary.Config, empty.Config}},
		instances: map[string]*InstanceStatus{
			"primary": primary,
			"empty":   empty,
		},
	}

	status := m.BackupStatus("godump", "1.2.3", now)
	if status.Overall != "failing" {
		t.Fatalf("overall %s", status.Overall)
	}
	if status.GeneratedAt != "2026-06-02T12:00:00Z" {
		t.Fatalf("generated %s", status.GeneratedAt)
	}
	if len(status.Jobs) != 3 {
		t.Fatalf("jobs %d", len(status.Jobs))
	}
	if status.Jobs[0].ID != "primary/app" || status.Jobs[1].ID != "primary/zeta" || status.Jobs[2].ID != "empty" {
		t.Fatalf("order %+v %+v %+v", status.Jobs[0].ID, status.Jobs[1].ID, status.Jobs[2].ID)
	}

	app := status.Jobs[0]
	if app.LastStatus != "failed" || app.Target != "10.0.0.5:3306/app" || app.LastError == nil || *app.LastError != "dump failed" {
		t.Fatalf("app job %+v", app)
	}
	if app.LastSuccessAt != nil || app.Stale {
		t.Fatalf("failed job should not invent a success: %+v", app)
	}

	zeta := status.Jobs[1]
	if zeta.LastStatus != "success" || !zeta.Stale || zeta.LastSizeBytes != 99 || zeta.LastDurationSeconds != 2 {
		t.Fatalf("zeta job %+v", zeta)
	}
	if zeta.NextRunAt == nil || *zeta.NextRunAt != next.Format(time.RFC3339) {
		t.Fatalf("next %v", zeta.NextRunAt)
	}

	if status.Jobs[2].LastStatus != "never_run" || status.Jobs[2].Target != "10.0.0.6:3307" {
		t.Fatalf("empty job %+v", status.Jobs[2])
	}

	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	jobs := decoded["jobs"].([]any)
	first := jobs[0].(map[string]any)
	if _, ok := first["last_success_at"]; !ok {
		t.Fatal("missing last_success_at")
	}
	if first["last_success_at"] != nil {
		t.Fatalf("last_success_at %#v", first["last_success_at"])
	}
	for _, key := range []string{"id", "name", "target", "enabled", "last_run_at", "last_status", "last_success_at", "last_duration_seconds", "last_size_bytes", "last_error", "next_run_at", "stale"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestOverallStatus(t *testing.T) {
	cases := []struct {
		name string
		jobs []BackupJobStatus
		want string
	}{
		{name: "none", want: "unknown"},
		{name: "disabled only", jobs: []BackupJobStatus{{Enabled: false, LastStatus: "failed"}}, want: "unknown"},
		{name: "ok", jobs: []BackupJobStatus{{Enabled: true, LastStatus: "success"}}, want: "ok"},
		{name: "running", jobs: []BackupJobStatus{{Enabled: true, LastStatus: "running"}}, want: "ok"},
		{name: "stale", jobs: []BackupJobStatus{{Enabled: true, LastStatus: "success", Stale: true}}, want: "warning"},
		{name: "partial", jobs: []BackupJobStatus{{Enabled: true, LastStatus: "partial"}}, want: "warning"},
		{name: "never", jobs: []BackupJobStatus{{Enabled: true, LastStatus: "never_run"}}, want: "warning"},
		{name: "failed wins", jobs: []BackupJobStatus{
			{Enabled: true, LastStatus: "success", Stale: true},
			{Enabled: true, LastStatus: "failed"},
		}, want: "failing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := overallStatus(tc.jobs); got != tc.want {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestStaleThreshold(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	hourly := "0 * * * *"
	if isStale(now.Add(-2*time.Hour), hourly, now) {
		t.Fatal("exactly two intervals should not be stale")
	}
	if !isStale(now.Add(-2*time.Hour-time.Second), hourly, now) {
		t.Fatal("older than two intervals should be stale")
	}
	if isStale(now.Add(-47*time.Hour), "", now) {
		t.Fatal("under 48h without a schedule should not be stale")
	}
	if !isStale(now.Add(-48*time.Hour-time.Second), "not cron", now) {
		t.Fatal("unparseable schedule should use 48h")
	}
	if isStale(time.Time{}, hourly, now) {
		t.Fatal("missing success is not stale")
	}
}

func TestDiscoveryFailureKeepsLastSuccess(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	last := now.Add(-time.Hour)
	inst := &InstanceStatus{
		Config: config.InstanceConfig{
			Name:     "primary",
			Host:     "db.internal",
			Port:     3306,
			Schedule: "0 2 * * *",
		},
		LastRunTime:    now,
		OverallResult:  "failed",
		DiscoveryError: "dial tcp: connection refused",
		Databases: map[string]*DBStatus{
			"app": {
				Name:             "app",
				LastBackupTime:   last,
				LastBackupResult: "success",
				LastBackupSize:   5,
			},
		},
	}
	m := &Manager{
		cfg:       &config.Config{Instances: []config.InstanceConfig{inst.Config}},
		instances: map[string]*InstanceStatus{"primary": inst},
	}
	job := m.BackupStatus("godump", "dev", now).Jobs[0]
	if job.LastStatus != "failed" || job.LastError == nil || *job.LastError != "dial tcp: connection refused" {
		t.Fatalf("%+v", job)
	}
	if job.LastSuccessAt == nil || *job.LastSuccessAt != last.Format(time.RFC3339) {
		t.Fatalf("success %v", job.LastSuccessAt)
	}
	if job.LastRunAt == nil || *job.LastRunAt != now.Format(time.RFC3339) {
		t.Fatalf("run %v", job.LastRunAt)
	}
	if m.BackupStatus("godump", "dev", now).Overall != "failing" {
		t.Fatal("expected failing")
	}
}

func TestRunningDatabase(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	last := now.Add(-time.Minute)
	inst := &InstanceStatus{
		Config:      config.InstanceConfig{Name: "primary", Host: "127.0.0.1", Port: 3306, Schedule: "0 * * * *"},
		IsRunning:   true,
		NextRunTime: now.Add(time.Hour),
		Databases: map[string]*DBStatus{
			"app": {
				Name:             "app",
				InProgress:       true,
				RunStartedAt:     now.Add(-2 * time.Second),
				LastBackupTime:   last,
				LastBackupResult: "success",
				LastBackupSize:   8,
				LastError:        "old error",
			},
		},
	}
	m := &Manager{
		cfg:       &config.Config{Instances: []config.InstanceConfig{inst.Config}},
		instances: map[string]*InstanceStatus{"primary": inst},
	}
	status := m.BackupStatus("godump", "dev", now)
	job := status.Jobs[0]
	if job.LastStatus != "running" || job.LastError != nil || status.Overall != "ok" {
		t.Fatalf("%+v overall %s", job, status.Overall)
	}
	if job.LastSuccessAt == nil || job.LastSizeBytes != 8 {
		t.Fatalf("%+v", job)
	}
}
