package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: xml-dump <file.vsdx> [path-pattern]")
		os.Exit(1)
	}

	r, err := zip.OpenReader(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer r.Close()

	pattern := ""
	if len(os.Args) > 2 {
		pattern = os.Args[2]
	}

	for _, f := range r.File {
		if pattern != "" && !strings.Contains(f.Name, pattern) {
			continue
		}

		if strings.HasSuffix(f.Name, ".xml") {
			fmt.Printf("=== %s ===\n", f.Name)
			rc, err := f.Open()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", f.Name, err)
				continue
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			fmt.Println(string(data))
			fmt.Println()
		} else if pattern == "" {
			fmt.Printf("  %s (%d bytes)\n", f.Name, f.UncompressedSize64)
		}
	}
}
