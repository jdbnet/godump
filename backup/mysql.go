package backup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"godump/config"
	"godump/logger"
)

func backupDatabase(cfg config.InstanceConfig, dbName string) (int64, error) {
	// Create backup directory for this database
	targetDir := filepath.Join(cfg.BackupDir, dbName)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create backup directory %s: %w", targetDir, err)
	}

	// Filename format: instancename_dbname_2006-01-02_15-04-05.sql.gz
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s_%s.sql.gz", cfg.Name, dbName, timestamp)
	targetFile := filepath.Join(targetDir, filename)

	writePath := targetFile
	if cfg.TempDir != "" {
		if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
			return 0, fmt.Errorf("failed to create temp directory %s: %w", cfg.TempDir, err)
		}
		writePath = filepath.Join(cfg.TempDir, filename)
		defer func() {
			if err := os.Remove(writePath); err != nil && !os.IsNotExist(err) {
				logger.Warn(cfg.Name, "Failed to remove temp backup %s: %v", writePath, err)
			}
		}()
		logger.Info(cfg.Name, "Staging backup for %s at %s", dbName, writePath)
	}

	f, err := os.Create(writePath)
	if err != nil {
		return 0, fmt.Errorf("failed to create backup file %s: %w", writePath, err)
	}
	defer f.Close()

	// Using sh -c allows us to pipe mysqldump output to gzip easily.
	// Since we are taking dbName from INFORMATION_SCHEMA, we should still be careful with quotes,
	// but mysqldump arguments can be passed via command directly, and we can pipe in go, or use sh.
	// Piping in Go is safer.

	cmdDump := exec.Command("mysqldump",
		fmt.Sprintf("-h%s", cfg.Host),
		fmt.Sprintf("-P%d", cfg.Port),
		fmt.Sprintf("-u%s", cfg.User),
		fmt.Sprintf("-p%s", cfg.Password),
		"--single-transaction",
		"--routines",
		"--triggers",
		dbName,
	)

	cmdGzip := exec.Command("gzip", "-c")

	// Create pipe between mysqldump and gzip
	dumpOut, err := cmdDump.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create dump stdout pipe: %w", err)
	}
	cmdDump.Stderr = os.Stderr // or capture it

	cmdGzip.Stdin = dumpOut
	cmdGzip.Stdout = f
	cmdGzip.Stderr = os.Stderr

	if err := cmdDump.Start(); err != nil {
		return 0, fmt.Errorf("failed to start mysqldump: %w", err)
	}

	if err := cmdGzip.Start(); err != nil {
		cmdDump.Process.Kill()
		return 0, fmt.Errorf("failed to start gzip: %w", err)
	}

	errGzip := cmdGzip.Wait()
	errDump := cmdDump.Wait()

	if errDump != nil {
		return 0, fmt.Errorf("mysqldump failed: %w", errDump)
	}

	if errGzip != nil {
		return 0, fmt.Errorf("gzip failed: %w", errGzip)
	}

	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("failed to close backup file %s: %w", writePath, err)
	}

	if writePath != targetFile {
		logger.Info(cfg.Name, "Copying backup for %s to %s", dbName, targetFile)
		if err := copyFile(writePath, targetFile); err != nil {
			if rmErr := os.Remove(targetFile); rmErr != nil && !os.IsNotExist(rmErr) {
				logger.Warn(cfg.Name, "Failed to remove partial backup %s: %v", targetFile, rmErr)
			}
			return 0, fmt.Errorf("failed to copy backup to %s: %w", targetFile, err)
		}
	}

	stat, err := os.Stat(targetFile)
	if err != nil {
		logger.Warn(cfg.Name, "Could not stat file %s to get size: %v", targetFile, err)
		return 0, nil
	}

	return stat.Size(), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}
