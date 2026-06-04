package backup

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"godump/config"
	"godump/logger"
)

func enforceRetention(cfg config.InstanceConfig) (int, error) {
	if cfg.RetentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -cfg.RetentionDays)
	deletedCount := 0

	err := filepath.WalkDir(cfg.BackupDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		if info.ModTime().Before(cutoff) {
			ageDays := int(time.Since(info.ModTime()).Hours() / 24)
			if err := os.Remove(path); err != nil {
				logger.Error(cfg.Name, "Failed to delete old backup %s: %v", path, err)
			} else {
				logger.Info(cfg.Name, "Deleted old backup %s (age: %d days)", path, ageDays)
				deletedCount++
			}
		}

		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return deletedCount, fmt.Errorf("error walking backup directory: %w", err)
	}

	return deletedCount, nil
}
