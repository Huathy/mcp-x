//go:build !windows

package docconv

// ComProbe on non-Windows always returns empty (no COM available), so the
// COM-backed tools are never registered.
func ComProbe(progIDHint string) string { return "" }

// WordToPdfCOM on non-Windows is a stub.
func WordToPdfCOM(src, dst, progID string, timeoutSec int) error {
	return ErrCOMNotAvailable
}

// PdfToWordCOM on non-Windows is a stub.
func PdfToWordCOM(src, dst, progID string, timeoutSec int) error {
	return ErrCOMNotAvailable
}
