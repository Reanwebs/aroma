package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := os.MkdirAll(filepath.Join("dist", "web"), 0o755); err != nil {
		fatal(err)
	}
	files := [][2]string{
		{filepath.Join("web", "index.html"), filepath.Join("dist", "index.html")},
		{filepath.Join("web", "app.css"), filepath.Join("dist", "web", "app.css")},
		{filepath.Join("web", "app.js"), filepath.Join("dist", "web", "app.js")},
	}
	for _, file := range files {
		content, err := os.ReadFile(file[0])
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(file[1], content, 0o644); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
