package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/modregistry"
	"github.com/3lbits/vigil/internal/testutil"
)

type adminExtraQ struct {
	testutil.StubQuerier
	setOrgErr            error
	deleteSessionsErr    error
	userByEmail          db.User
	userByEmailErr       error
	setRoleErr           error
	preCreateErr         error
	deleteUserErr        error
	createOrgErr         error
	deleteOrgErr         error
	updateRiskErr        error
	getAppSettings       db.AppSetting
	getAppSettingsErr    error
	updateAppSettingsErr error
	upsertCalls          int
}

func (q *adminExtraQ) SetUserOrg(_ context.Context, _ db.SetUserOrgParams) (db.User, error) {
	return db.User{}, q.setOrgErr
}
func (q *adminExtraQ) DeleteSessionsByUserID(_ context.Context, _ uuid.NullUUID) error {
	return q.deleteSessionsErr
}
func (q *adminExtraQ) GetUserByEmail(_ context.Context, _ string) (db.User, error) {
	return q.userByEmail, q.userByEmailErr
}
func (q *adminExtraQ) SetUserRole(_ context.Context, _ db.SetUserRoleParams) (db.User, error) {
	return db.User{}, q.setRoleErr
}
func (q *adminExtraQ) PreCreateUser(_ context.Context, _ db.PreCreateUserParams) (db.User, error) {
	return db.User{}, q.preCreateErr
}
func (q *adminExtraQ) DeleteUser(_ context.Context, _ uuid.UUID) error {
	return q.deleteUserErr
}
func (q *adminExtraQ) CreateOrganization(_ context.Context, _ db.CreateOrganizationParams) (db.Organization, error) {
	return db.Organization{ID: uuid.New(), Name: "Org"}, q.createOrgErr
}
func (q *adminExtraQ) DeleteOrganization(_ context.Context, _ uuid.UUID) error {
	return q.deleteOrgErr
}
func (q *adminExtraQ) UpdateRiskGlobalSettings(_ context.Context, _ db.UpdateRiskGlobalSettingsParams) error {
	return q.updateRiskErr
}
func (q *adminExtraQ) UpsertRiskScaleLabel(_ context.Context, _ db.UpsertRiskScaleLabelParams) error {
	q.upsertCalls++
	return nil
}
func (q *adminExtraQ) GetAppSettings(_ context.Context) (db.AppSetting, error) {
	return q.getAppSettings, q.getAppSettingsErr
}
func (q *adminExtraQ) UpdateAppSettings(_ context.Context, _ db.UpdateAppSettingsParams) error {
	return q.updateAppSettingsErr
}

func newExtraHandler(q *adminExtraQ) *Handler {
	return NewHandler(q, noopPinger{}, time.Now(), "test", nil)
}

func newExtraHandlerWithRefresh(q *adminExtraQ, refresh func(context.Context) error) *Handler {
	return NewHandler(q, noopPinger{}, time.Now(), "test", refresh)
}

func TestFilterAuditLog(t *testing.T) {
	rows := []db.ListAuditLogAdminRow{
		{Event: "admin.user.delete"},
		{Event: "admin.user.update"},
		{Event: "admin.user.create"},
	}
	if got := filterAuditLog(rows, "all"); len(got) != 3 {
		t.Fatalf("all filter expected 3 rows, got %d", len(got))
	}
	if got := filterAuditLog(rows, "delete"); len(got) != 1 || got[0].Event != "admin.user.delete" {
		t.Fatalf("delete filter mismatch: %#v", got)
	}
	if got := filterAuditLog(rows, "update"); len(got) != 1 || got[0].Event != "admin.user.update" {
		t.Fatalf("update filter mismatch: %#v", got)
	}
}

func TestParseInt32(t *testing.T) {
	if got := parseInt32("", 5); got != 5 {
		t.Fatalf("expected default 5, got %d", got)
	}
	if got := parseInt32("-1", 5); got != 5 {
		t.Fatalf("negative value should fallback to default, got %d", got)
	}
	if got := parseInt32("9", 5); got != 9 {
		t.Fatalf("expected parsed value 9, got %d", got)
	}
}

func TestFormatConsequenceTopics(t *testing.T) {
	out := formatConsequenceTopics("ops", "data", "", "trust")
	if !strings.Contains(out, "Financial / Operational: ops") ||
		!strings.Contains(out, "Confidentiality / Data: data") ||
		!strings.Contains(out, "Reputation / Trust: trust") {
		t.Fatalf("unexpected formatting: %q", out)
	}
}

func TestSetUserOrg_InvalidOrgID(t *testing.T) {
	h := newExtraHandler(&adminExtraQ{})
	uid := uuid.New()
	form := url.Values{"org_id": {"not-a-uuid"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users/"+uid.String()+"/org", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", uid.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SetUserOrg(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetUserOrg_OK(t *testing.T) {
	h := newExtraHandler(&adminExtraQ{})
	uid := uuid.New()
	form := url.Values{"org_id": {uuid.NewString()}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users/"+uid.String()+"/org", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", uid.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SetUserOrg(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
}

func TestRevokeUserSessions_OK(t *testing.T) {
	h := newExtraHandler(&adminExtraQ{})
	uid := uuid.New()
	r := httptest.NewRequest(http.MethodPost, "/admin/sessions/"+uid.String()+"/revoke", nil)
	r.SetPathValue("id", uid.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.RevokeUserSessions(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
}

func TestPreCreateUser_ExistingUserUpdatesRole(t *testing.T) {
	q := &adminExtraQ{userByEmail: db.User{ID: uuid.New(), Provider: "entra_id"}}
	h := newExtraHandler(q)
	form := url.Values{"email": {"a@example.com"}, "name": {"Alice"}, "role": {"editor"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.PreCreateUser(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "role+updated") {
		t.Fatalf("expected role updated redirect, got %q", w.Header().Get("Location"))
	}
}

func TestPreCreateUser_EmailRequired(t *testing.T) {
	h := newExtraHandler(&adminExtraQ{})
	form := url.Values{"email": {""}, "role": {"viewer"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.PreCreateUser(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
}

func TestDeleteUser_SelfDeleteBlocked(t *testing.T) {
	h := newExtraHandler(&adminExtraQ{})
	uid := "00000000-0000-0000-0000-000000000001"
	r := httptest.NewRequest(http.MethodPost, "/admin/users/"+uid+"/delete", nil)
	r.SetPathValue("id", uid)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.DeleteUser(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "Cannot+delete+your+own+account") {
		t.Fatalf("unexpected location: %q", w.Header().Get("Location"))
	}
}

func TestCreateOrgAndDeleteOrg_OK(t *testing.T) {
	h := newExtraHandler(&adminExtraQ{})
	form := url.Values{"name": {"Org One"}, "key": {"org-1"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/orgs", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()
	h.CreateOrg(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("create expected 303, got %d", w.Code)
	}

	id := uuid.New()
	r2 := httptest.NewRequest(http.MethodPost, "/admin/orgs/"+id.String()+"/delete", nil)
	r2.SetPathValue("id", id.String())
	r2 = adminCtx(r2)
	w2 := httptest.NewRecorder()
	h.DeleteOrg(w2, r2)
	if w2.Code != http.StatusSeeOther {
		t.Fatalf("delete expected 303, got %d", w2.Code)
	}
}

func TestSaveRiskSettings_OK(t *testing.T) {
	q := &adminExtraQ{}
	h := newExtraHandler(q)
	form := url.Values{
		"acceptance_criteria": {"criteria"},
		"low_max":             {"4"},
		"high_min":            {"10"},
		"prob_label_1":        {"Rare"},
		"cons_finops_desc_1":  {"Low"},
	}
	r := httptest.NewRequest(http.MethodPost, "/admin/risk-settings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SaveRiskSettings(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if q.upsertCalls == 0 {
		t.Fatal("expected risk scale upsert calls")
	}
}

func TestSaveModuleSettings_GetCurrentError(t *testing.T) {
	h := newExtraHandler(&adminExtraQ{getAppSettingsErr: errors.New("db down")})
	form := url.Values{"risk_enabled": {"on"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/module-settings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.SaveModuleSettings(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error") {
		t.Fatalf("expected error flash redirect, got %q", w.Header().Get("Location"))
	}
}

func TestRefreshModuleFlagsCalledOnSuccessfulWrites(t *testing.T) {
	refreshCalls := 0
	refresh := func(context.Context) error {
		refreshCalls++
		return nil
	}
	h := newExtraHandlerWithRefresh(&adminExtraQ{}, refresh)

	createForm := url.Values{"name": {"Org One"}, "key": {"org-1"}}
	createReq := httptest.NewRequest(http.MethodPost, "/admin/orgs", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq = adminCtx(createReq)
	createW := httptest.NewRecorder()
	h.CreateOrg(createW, createReq)
	if createW.Code != http.StatusSeeOther {
		t.Fatalf("create expected 303, got %d", createW.Code)
	}

	deleteID := uuid.New()
	deleteReq := httptest.NewRequest(http.MethodPost, "/admin/orgs/"+deleteID.String()+"/delete", nil)
	deleteReq.SetPathValue("id", deleteID.String())
	deleteReq = adminCtx(deleteReq)
	deleteW := httptest.NewRecorder()
	h.DeleteOrg(deleteW, deleteReq)
	if deleteW.Code != http.StatusSeeOther {
		t.Fatalf("delete expected 303, got %d", deleteW.Code)
	}

	moduleForm := url.Values{
		"compliance_enabled": {"on"},
		"risk_enabled":       {"on"},
		"activities_enabled": {"on"},
		"assets_enabled":     {"on"},
		"avvik_enabled":      {"on"},
	}
	moduleReq := httptest.NewRequest(http.MethodPost, "/admin/module-settings", strings.NewReader(moduleForm.Encode()))
	moduleReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	moduleReq = adminCtx(moduleReq)
	moduleW := httptest.NewRecorder()
	h.SaveModuleSettings(moduleW, moduleReq)
	if moduleW.Code != http.StatusSeeOther {
		t.Fatalf("module settings expected 303, got %d", moduleW.Code)
	}

	if refreshCalls != 3 {
		t.Fatalf("expected refresh callback 3 times, got %d", refreshCalls)
	}
}

func TestModuleContract(t *testing.T) {
	m := New(noopPinger{}, time.Now(), "test", nil)
	if got := m.Name(); got != "admin" {
		t.Fatalf("module name = %q, want %q", got, "admin")
	}
	mux := http.NewServeMux()
	r := modregistry.NewRegistrar(mux, nil)
	if err := m.Register(modregistry.Dependencies{}, r); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if got := len(r.Meta()); got == 0 {
		t.Fatal("expected routes to be registered")
	}
}
