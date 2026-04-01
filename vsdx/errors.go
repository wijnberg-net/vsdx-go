package vsdx

import (
	"errors"
	"fmt"
)

// Sentinel errors for common error conditions.
var (
	ErrInvalidFileType = errors.New("invalid file type: expected .vsdx, .vsdm, .vssx, or .vssm")
	ErrInvalidFormat   = errors.New("invalid vsdx format")
	ErrShapeNotFound   = errors.New("shape not found")
)

// FileError wraps an error with the associated file path.
type FileError struct {
	Path string
	Err  error
}

func (e *FileError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Err)
}

func (e *FileError) Unwrap() error {
	return e.Err
}
