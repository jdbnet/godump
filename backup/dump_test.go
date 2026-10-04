package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLookupDumpBinaryPrefersMariaDB(t *testing.T) {
	got, err := lookupDumpBinary(func(name string) (string, error) {
		switch name {
		case "mariadb-dump":
			return "/usr/bin/mariadb-dump", nil
		case "mysqldump":
			return "/usr/bin/mysqldump", nil
		default:
			return "", errors.New("missing")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/usr/bin/mariadb-dump" {
		t.Fatalf("got %q", got)
	}
}

func TestLookupDumpBinaryFallsBackToMySQLDump(t *testing.T) {
	got, err := lookupDumpBinary(func(name string) (string, error) {
		if name == "mysqldump" {
			return "/usr/bin/mysqldump", nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/usr/bin/mysqldump" {
		t.Fatalf("got %q", got)
	}
}

func TestLookupDumpBinaryMissing(t *testing.T) {
	_, err := lookupDumpBinary(func(string) (string, error) {
		return "", errors.New("missing")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLimitedBufferCapsGrowth(t *testing.T) {
	var buf limitedBuffer
	chunk := make([]byte, 1024)
	for i := 0; i < 100; i++ {
		n, err := buf.Write(chunk)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(chunk) {
			t.Fatalf("write length %d", n)
		}
	}
	if buf.buf.Len() != maxStderrBytes {
		t.Fatalf("buffer len %d, want %d", buf.buf.Len(), maxStderrBytes)
	}
}

func TestDropFileCache(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "backup.sql.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(make([]byte, 8192)); err != nil {
		t.Fatal(err)
	}
	dropFileCache(f)
}
