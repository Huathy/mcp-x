//go:build !windows

package docconv

// comProbe on non-Windows always returns empty (no COM available).
func comProbe(progIDHint string) string { return "" }

// wordToPdfCOM on non-Windows is a stub.
func wordToPdfCOM(src, dst, progID string, timeoutSec int) error {
	return ErrCOMNotAvailable
}

// pdfToWordCOM on non-Windows is a stub.
func pdfToWordCOM(src, dst, progID string, timeoutSec int) error {
	return ErrCOMNotAvailable
}
