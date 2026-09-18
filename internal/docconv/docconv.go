package docconv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathOutsideWorkdir = errors.New("path is outside docconv workdir")
	ErrPathTraversal      = errors.New("path must not contain '..'")
	ErrCOMNotAvailable    = errors.New("Word/PDF 互转需 Windows + MS Office/WPS")
	ErrFileTooLarge       = errors.New("input file exceeds max_file_size_mb")
	ErrScanPDF            = errors.New("PDF 无文本层（可能为扫描件），需 OCR")
)

// Config mirrors the docconv-related fields a converter needs at runtime.
type Config struct {
	Workdir       string
	MaxFileSizeMB int
	ComProgID     string
	ComTimeoutSec int
}

// ValidatePath ensures p resolves inside workdir and has no traversal.
func ValidatePath(workdir, p string) error {
	if strings.Contains(filepath.ToSlash(p), "..") {
		return ErrPathTraversal
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	wd, err := filepath.Abs(workdir)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(abs, wd+string(filepath.Separator)) && abs != wd {
		return ErrPathOutsideWorkdir
	}
	return nil
}

// CheckSize verifies a file does not exceed the configured size limit.
func CheckSize(path string, maxMB int) error {
	if maxMB <= 0 {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat input: %w", err)
	}
	if fi.Size() > int64(maxMB)*1024*1024 {
		return fmt.Errorf("%w: %d bytes (limit %dMB)", ErrFileTooLarge, fi.Size(), maxMB)
	}
	return nil
}
