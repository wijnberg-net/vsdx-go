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

	// Formula evaluation errors
	ErrFormulaUnsupported   = errors.New("formula function not implemented")
	ErrFormulaInvalidSyntax = errors.New("formula syntax error")
	ErrFormulaCycleDetected = errors.New("circular reference detected in formula")
	ErrFormulaDivByZero     = errors.New("division by zero in formula")

	// OPC validation errors
	ErrMissingRequiredPart   = errors.New("missing required OPC part")
	ErrInvalidContentType    = errors.New("invalid or missing content type")
	ErrInvalidRelationship   = errors.New("invalid relationship")
	ErrOrphanedPart          = errors.New("orphaned part (no relationship)")
	ErrDuplicateRelationship = errors.New("duplicate relationship ID")
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
