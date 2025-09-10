package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func mustHave(bin string) error {
	_, err := exec.LookPath(bin)
	return err
}

func baseNoExt(p string) string {
	dir := filepath.Dir(p)
	name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	return filepath.Join(dir, name)
}

func findSingleChildDir(root string) (string, error) {
	f, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range f {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) != 1 {
		return "", errors.New("expected exactly one subdir")
	}
	return dirs[0], nil
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "[-] "+format+"\n", a...)
	os.Exit(1)
}
