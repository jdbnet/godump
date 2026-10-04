package backup

import (
	"bytes"
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

	dumpBin, err := dumpBinary()
	if err != nil {
		return 0, err
	}

	cmdDump := exec.Command(dumpBin,
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

	dumpOut, err := cmdDump.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create dump stdout pipe: %w", err)
	}
	var dumpStderr, gzipStderr limitedBuffer
	cmdDump.Stderr = &dumpStderr
	cmdGzip.Stdin = dumpOut
	cmdGzip.Stdout = f
	cmdGzip.Stderr = &gzipStderr

	if err := cmdDump.Start(); err != nil {
		return 0, fmt.Errorf("failed to start %s: %w", dumpBin, err)
	}
	dumpWaited := false
	defer func() {
		if !dumpWaited && cmdDump.Process != nil {
			_ = cmdDump.Process.Kill()
			_ = cmdDump.Wait()
		}
	}()

	if err := cmdGzip.Start(); err != nil {
		return 0, fmt.Errorf("failed to start gzip: %w", err)
	}
	gzipWaited := false
	defer func() {
		if !gzipWaited && cmdGzip.Process != nil {
			_ = cmdGzip.Process.Kill()
			_ = cmdGzip.Wait()
		}
	}()

	errGzip := cmdGzip.Wait()
	gzipWaited = true
	errDump := cmdDump.Wait()
	dumpWaited = true

	if errDump != nil {
		return 0, fmt.Errorf("%s failed: %w%s", dumpBin, errDump, stderrSuffix(dumpStderr.String()))
	}

	if errGzip != nil {
		return 0, fmt.Errorf("gzip failed: %w%s", errGzip, stderrSuffix(gzipStderr.String()))
	}

	// Written backup pages stay in the kernel cache and Docker counts them as
	// container memory long after the job has finished. Drop them once the
	// file is durable.
	dropFileCache(f)
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
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	dropFileCache(in)
	dropFileCache(out)

	if err := in.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

const maxStderrBytes = 8192

type limitedBuffer struct {
	buf bytes.Buffer
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.buf.Len() < maxStderrBytes {
		remain := maxStderrBytes - l.buf.Len()
		chunk := p
		if len(chunk) > remain {
			chunk = chunk[:remain]
		}
		_, _ = l.buf.Write(chunk)
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

func stderrSuffix(msg string) string {
	msg = string(bytes.TrimSpace([]byte(msg)))
	if msg == "" {
		return ""
	}
	return ": " + msg
}
