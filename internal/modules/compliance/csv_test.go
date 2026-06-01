package compliance

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/google/uuid"
)

// ── CSV-specific test double ──────────────────────────────────────────────────

type csvQ struct {
	testutil.StubQuerier
	created   []db.CreateRequirementParams
	createErr error
	// Fail only after this many successful creates.
	failAfter int
	callCount int
}

func (q *csvQ) ListFrameworks(_ context.Context) ([]db.Framework, error) {
	return nil, nil
}
func (q *csvQ) CountRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (q *csvQ) CountCoveredRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (q *csvQ) ListRequirementsByFramework(_ context.Context, _ uuid.UUID) ([]db.Requirement, error) {
	return nil, nil
}
func (q *csvQ) ListMeasuresForRequirement(_ context.Context, _ uuid.UUID) ([]db.ListMeasuresForRequirementRow, error) {
	return nil, nil
}
func (q *csvQ) CreateRequirement(_ context.Context, p db.CreateRequirementParams) (db.Requirement, error) {
	q.callCount++
	if q.createErr != nil && (q.failAfter == 0 || q.callCount > q.failAfter) {
		return db.Requirement{}, q.createErr
	}
	q.created = append(q.created, p)
	return db.Requirement{}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildMultipart creates a multipart/form-data request containing the given CSV.
func buildMultipartRequest(t *testing.T, fwID, csvContent string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("csv_file", "requirements.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(csvContent)); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/"+fwID+"/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.SetPathValue("id", fwID)
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Alice", Role: "admin",
	})
	return r.WithContext(ctx)
}

// ── parseAndImportCSV unit tests ──────────────────────────────────────────────

func TestParseAndImportCSV_ValidCSV(t *testing.T) {
	q := &csvQ{}
	fwID := uuid.New()
	csv := "ref,title,description,sort_order\nA.1,Access control,Restrict access,10\nA.2,Logging,Enable audit logs,20\n"

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, q, fwID, strings.NewReader(csv))

	if result.Count != 2 {
		t.Errorf("expected 2 imported rows, got %d", result.Count)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
	if q.created[0].Ref != "A.1" {
		t.Errorf("expected first ref A.1, got %q", q.created[0].Ref)
	}
	if q.created[0].SortOrder != 10 {
		t.Errorf("expected sort_order 10, got %d", q.created[0].SortOrder)
	}
}

func TestParseAndImportCSV_MissingRefColumn(t *testing.T) {
	csv := "title,description\nAccess control,Restrict access\n"
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, &csvQ{}, uuid.New(), strings.NewReader(csv))

	if len(result.Errors) == 0 {
		t.Error("expected error for missing 'ref' column")
	}
	if result.Count != 0 {
		t.Errorf("expected 0 rows imported, got %d", result.Count)
	}
}

func TestParseAndImportCSV_MissingTitleColumn(t *testing.T) {
	csv := "ref,description\nA.1,Restrict access\n"
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, &csvQ{}, uuid.New(), strings.NewReader(csv))

	if len(result.Errors) == 0 {
		t.Error("expected error for missing 'title' column")
	}
}

func TestParseAndImportCSV_EmptyRefOrTitle(t *testing.T) {
	csv := "ref,title\nA.1,\n,Missing ref\n"
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, &csvQ{}, uuid.New(), strings.NewReader(csv))

	if len(result.Errors) != 2 {
		t.Errorf("expected 2 row errors, got %d: %v", len(result.Errors), result.Errors)
	}
	if result.Count != 0 {
		t.Errorf("expected 0 rows imported, got %d", result.Count)
	}
}

func TestParseAndImportCSV_MinimalColumns(t *testing.T) {
	// Only required columns, no description or sort_order.
	csv := "ref,title\nB.1,Backup policy\nB.2,Restore testing\n"
	q := &csvQ{}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, q, uuid.New(), strings.NewReader(csv))

	if result.Count != 2 {
		t.Errorf("expected 2 rows, got %d", result.Count)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestParseAndImportCSV_DBErrorOnRow(t *testing.T) {
	q := &csvQ{createErr: errors.New("unique violation")}
	csv := "ref,title\nA.1,Title\n"
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, q, uuid.New(), strings.NewReader(csv))

	if len(result.Errors) == 0 {
		t.Error("expected error recorded for DB failure")
	}
	if result.Count != 0 {
		t.Errorf("expected 0 successful rows, got %d", result.Count)
	}
}

func TestParseAndImportCSV_PartialFailure(t *testing.T) {
	// First row succeeds, second fails.
	q := &csvQ{createErr: errors.New("db error"), failAfter: 1}
	csv := "ref,title\nA.1,First\nA.2,Second\n"
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, q, uuid.New(), strings.NewReader(csv))

	if result.Count != 1 {
		t.Errorf("expected 1 successful row, got %d", result.Count)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParseAndImportCSV_CaseInsensitiveHeaders(t *testing.T) {
	csv := "REF,TITLE\nC.1,Crypto\n"
	q := &csvQ{}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	result := parseAndImportCSV(r, q, uuid.New(), strings.NewReader(csv))

	if result.Count != 1 {
		t.Errorf("expected 1 row, got %d (headers should be case-insensitive)", result.Count)
	}
}

// ── Import handler integration ────────────────────────────────────────────────

func TestImport_Handler_ValidCSV(t *testing.T) {
	q := &csvQ{}
	h := NewHandler(q, nil)
	fwID := uuid.New()

	csv := "ref,title\nD.1,Data classification\n"
	r := buildMultipartRequest(t, fwID.String(), csv)
	w := httptest.NewRecorder()

	h.Import(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if q.callCount != 1 {
		t.Errorf("expected 1 CreateRequirement call, got %d", q.callCount)
	}
}

func TestImport_Handler_InvalidFrameworkID(t *testing.T) {
	h := NewHandler(&csvQ{}, nil)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/bad-id/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.SetPathValue("id", "bad-id")
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{Role: "admin"})
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()

	h.Import(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestImport_Handler_MissingFile(t *testing.T) {
	h := NewHandler(&csvQ{}, nil)
	fwID := uuid.New()

	// Build multipart without a file field.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("other_field", "value")
	_ = mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/"+fwID.String()+"/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.SetPathValue("id", fwID.String())
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{Role: "admin"})
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()

	h.Import(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
