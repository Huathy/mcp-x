//go:build windows

package docconv

import (
	"fmt"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// ComProbe tries each candidate ProgID and returns the first that creates.
// progIDHint is "auto" or an explicit ProgID.
func ComProbe(progIDHint string) string {
	candidates := []string{"Word.Application", "kwps.Application", "wps.Application"}
	if progIDHint != "" && progIDHint != "auto" {
		candidates = append([]string{progIDHint}, candidates...)
	}
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	defer ole.CoUninitialize()
	for _, p := range candidates {
		unk, err := oleutil.CreateObject(p)
		if err != nil {
			continue
		}
		unk.Release()
		return p
	}
	return ""
}

// WordToPdfCOM uses MS Word / WPS to export docx → pdf.
func WordToPdfCOM(src, dst, progID string, timeoutSec int) error {
	return withCOM(progID, timeoutSec, func(app *ole.IDispatch) error {
		docs, err := oleutil.GetProperty(app, "Documents")
		if err != nil {
			return fmt.Errorf("get Documents: %w", err)
		}
		defer docs.Clear()
		doc, err := oleutil.CallMethod(docs.ToIDispatch(), "Open", src)
		if err != nil {
			return fmt.Errorf("open %s: %w", src, err)
		}
		defer doc.Clear()
		if _, err := oleutil.CallMethod(doc.ToIDispatch(), "SaveAs2", dst, 17); err != nil {
			return fmt.Errorf("saveas pdf: %w", err)
		}
		if _, err := oleutil.CallMethod(doc.ToIDispatch(), "Close", false); err != nil {
			return fmt.Errorf("close doc: %w", err)
		}
		return nil
	})
}

// PdfToWordCOM uses MS Word to import pdf → docx. Word-only; WPS not
// guaranteed.
func PdfToWordCOM(src, dst, progID string, timeoutSec int) error {
	if progID != "Word.Application" {
		return fmt.Errorf("pdf_to_word only supported with MS Word (got %s)", progID)
	}
	return withCOM(progID, timeoutSec, func(app *ole.IDispatch) error {
		docs, err := oleutil.GetProperty(app, "Documents")
		if err != nil {
			return fmt.Errorf("get Documents: %w", err)
		}
		defer docs.Clear()
		doc, err := oleutil.CallMethod(docs.ToIDispatch(), "Open", src)
		if err != nil {
			return fmt.Errorf("open pdf %s: %w", src, err)
		}
		defer doc.Clear()
		if _, err := oleutil.CallMethod(doc.ToIDispatch(), "SaveAs2", dst, 12); err != nil {
			return fmt.Errorf("saveas docx: %w", err)
		}
		if _, err := oleutil.CallMethod(doc.ToIDispatch(), "Close", false); err != nil {
			return fmt.Errorf("close doc: %w", err)
		}
		return nil
	})
}

func withCOM(progID string, timeoutSec int, fn func(app *ole.IDispatch) error) error {
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	done := make(chan error, 1)
	go func() {
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		defer ole.CoUninitialize()
		unk, err := oleutil.CreateObject(progID)
		if err != nil {
			done <- fmt.Errorf("COM create %s: %w", progID, err)
			return
		}
		defer unk.Release()
		app, err := unk.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			done <- fmt.Errorf("query IDispatch: %w", err)
			return
		}
		defer app.Release()
		// Word needs to be visible=false to avoid stealing focus; set via property
		_, _ = oleutil.PutProperty(app, "Visible", false)
		err = fn(app)
		_, _ = oleutil.CallMethod(app, "Quit")
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		return fmt.Errorf("COM call timed out after %ds", timeoutSec)
	}
}
