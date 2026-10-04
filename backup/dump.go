package backup

import (
	"fmt"
	"os/exec"
)

func dumpBinary() (string, error) {
	return lookupDumpBinary(exec.LookPath)
}

func lookupDumpBinary(lookPath func(string) (string, error)) (string, error) {
	if path, err := lookPath("mariadb-dump"); err == nil {
		return path, nil
	}
	if path, err := lookPath("mysqldump"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("neither mariadb-dump nor mysqldump was found in PATH")
}
