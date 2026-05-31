package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// minimalPDF is a tiny but valid PDF containing an uncompressed content stream
// with a single text-showing operator.
const minimalPDF = "%PDF-1.4\n" +
	"1 0 obj\n<< /Length 44 >>\nstream\n" +
	"BT /F1 24 Tf (Hello World PDF text) Tj ET\n" +
	"endstream\nendobj\n%%EOF\n"

func TestExtractPDFHandler_NonPost(t *testing.T) {
	h := extractPDFHandler(&Deps{})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/extract-pdf", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestExtractPDFHandler_MissingURL(t *testing.T) {
	h := extractPDFHandler(&Deps{Cfg: Config{RequestTimeoutMs: 5000}})
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{})
	h(rec, httptest.NewRequest(http.MethodPost, "/extract-pdf", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestExtractPDFHandler_FetchAndExtract(t *testing.T) {
	// Serve the minimal PDF from a local test server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte(minimalPDF))
	}))
	defer srv.Close()

	deps := &Deps{
		Cfg:  Config{RequestTimeoutMs: 5000},
		HTTP: srv.Client(),
	}
	h := extractPDFHandler(deps)

	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"url": srv.URL})
	h(rec, httptest.NewRequest(http.MethodPost, "/extract-pdf", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp PDFExtractResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Contains([]byte(resp.Text), []byte("Hello World")) {
		t.Errorf("extracted text missing expected content: %q", resp.Text)
	}
	if resp.Chars != len(resp.Text) {
		t.Errorf("chars=%d != len(text)=%d", resp.Chars, len(resp.Text))
	}
}

func TestExtractPDFHandler_NotAPDF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not a pdf</html>"))
	}))
	defer srv.Close()

	deps := &Deps{Cfg: Config{RequestTimeoutMs: 5000}, HTTP: srv.Client()}
	h := extractPDFHandler(deps)

	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"url": srv.URL})
	h(rec, httptest.NewRequest(http.MethodPost, "/extract-pdf", bytes.NewReader(body)))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422 for non-PDF content, got %d", rec.Code)
	}
}
