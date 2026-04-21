package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wraelen/framescli/internal/contracts"
)

func main() {
	repoRoot, err := os.Getwd()
	if err != nil {
		fatalf("get working directory: %v", err)
	}

	files, err := contracts.GeneratedFiles()
	if err != nil {
		fatalf("render generated files: %v", err)
	}

	for _, file := range files {
		target := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fatalf("create directory for %s: %v", file.Path, err)
		}
		if err := os.WriteFile(target, file.Contents, 0o644); err != nil {
			fatalf("write %s: %v", file.Path, err)
		}
		fmt.Println(file.Path)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
