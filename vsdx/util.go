package vsdx

import (
	"os"
	"path/filepath"
)

// writeFileBytes writes data to a file, creating parent directories as needed.
func writeFileBytes(filename string, data []byte) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}
