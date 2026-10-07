package admintemplates

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
)

func TestAdminPage_UI2TabsAndForms(t *testing.T) {
	var out bytes.Buffer
	if err := AdminPage(nil, nil, nil, "test", time.Now(), true, "", "", "orgs", "all",
		nil, db.RiskGlobalSetting{}, nil, db.AppSetting{}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		`class="ui2 ui2-admin"`, `href="/admin?tab=orgs"`, `id="tab-panel-orgs"`,
		`action="/admin/orgs"`, `name="_csrf"`, `aria-label=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin page missing %q", want)
		}
	}
	if strings.Count(body, `class="ui2 ui2-admin"`) != 1 {
		t.Error("admin page must have exactly one UI2 root")
	}
}

func TestStandaloneAdminPages_UI2Roots(t *testing.T) {
	for name, page := range map[string]templ.Component{
		"organizations": OrgsPage(nil, "", ""),
		"risk settings": RiskSettingsPage("", "", ""),
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := page.Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}

			if strings.Count(out.String(), `class="ui2 ui2-admin"`) != 1 {
				t.Error("standalone admin page needs one UI2 root")
			}
			if !strings.Contains(out.String(), `name="_csrf"`) {
				t.Error("standalone admin form must keep its CSRF field")
			}
		})
	}
}

func TestAdminDestructiveActions_DangerTone(t *testing.T) {
	org := db.Organization{ID: uuid.New(), Name: "Team"}
	var out bytes.Buffer
	if err := AdminPage([]db.User{{ID: uuid.New(), Name: "Bob"}},
		[]db.ListActiveSessionsByUserRow{{UserID: uuid.New(), Name: "Bob"}}, nil,
		"test", time.Now(), true, "", "", "users", "all",
		[]db.Organization{org}, db.RiskGlobalSetting{}, nil, db.AppSetting{}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Find(`form[action$="/revoke"] button[data-tone="danger"]`).Length(); got != 1 {
		t.Errorf("expected revoke button with danger tone, got %d", got)
	}
	if got := doc.Find(`form[action$="/delete"] button[data-tone="danger"]`).Length(); got != 2 {
		t.Errorf("expected user and organization delete buttons with danger tone, got %d", got)
	}
}

func TestStandaloneAdminForms_NativeFieldsAndFlash(t *testing.T) {
	for _, tc := range []struct {
		name, action, field string
		page                templ.Component
	}{
		{"organizations", "/admin/orgs", "name", OrgsPage(nil, "Saved", "success")},
		{"risk settings", "/admin/risk-settings", "acceptance_criteria", RiskSettingsPage("Review annually", "Saved", "success")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tc.page.Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}

			doc, err := goquery.NewDocumentFromReader(bytes.NewReader(out.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			form := doc.Find(`form[action="` + tc.action + `"]`)
			if form.Find(`input[name="_csrf"]`).Length() != 1 ||
				form.Find(`[name="`+tc.field+`"]`).Length() != 1 {
				t.Error("admin form must retain its action, CSRF field, and named control")
			}
			if doc.Find(`.ui2-notice[role="status"][data-tone="success"]`).Text() != "Saved" {
				t.Error("success flash must retain its status announcement")
			}
		})
	}
}

func TestAdminPage_SemanticSettingsAndStatus(t *testing.T) {
	var out bytes.Buffer
	if err := AdminPage(nil, nil, nil, "test", time.Now(), true, "", "", "risk", "all",
		nil, db.RiskGlobalSetting{}, nil, db.AppSetting{}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Find(`[role="tabpanel"]`).Length(); got != 7 {
		t.Errorf("expected seven accessible tab panels, got %d", got)
	}
	doc.Find(`[role="tab"]`).Each(func(_ int, tab *goquery.Selection) {
		panelID, ok := tab.Attr("aria-controls")
		if !ok || panelID == "" || doc.Find("#"+panelID).AttrOr("aria-labelledby", "") != tab.AttrOr("id", "") {
			t.Errorf("tab %q must control a panel that labels itself with the tab", tab.Text())
		}
	})
	if got := doc.Find(`#tab-panel-users > section > h2, #tab-panel-orgs > section > h2, #tab-panel-status > div > section > h2`).Length(); got != 3 {
		t.Errorf("expected three section headings under their sections, got %d", got)
	}
	if got := doc.Find(`#tab-panel-risk fieldset > legend`).Length(); got != 3 {
		t.Errorf("expected three named risk settings groups, got %d", got)
	}
	if got := doc.Find(`#tab-panel-status dl dt`).Length(); got != 5 {
		t.Errorf("expected five labeled status values, got %d", got)
	}
	if got := doc.Find(`#tab-panel-audit nav a`).Length(); got != 3 {
		t.Errorf("expected three audit filter links in navigation, got %d", got)
	}
	if got := doc.Find(`#tab-panel-modules input[type="checkbox"][aria-label]`).Length(); got != 5 {
		t.Errorf("expected five named module switches, got %d", got)
	}
}

func TestAdminOrgLists_NestedAndLabeled(t *testing.T) {
	parent := db.Organization{ID: uuid.New(), Name: "Parent"}
	child := db.Organization{ID: uuid.New(), Name: "Child", ParentID: uuid.NullUUID{UUID: parent.ID, Valid: true}}
	for name, page := range map[string]templ.Component{
		"admin tab": AdminPage(nil, nil, nil, "test", time.Now(), true, "", "", "orgs", "all",
			[]db.Organization{parent, child}, db.RiskGlobalSetting{}, nil, db.AppSetting{}),
		"organizations page": OrgsPage([]db.Organization{parent, child}, "", ""),
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := page.Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			doc, err := goquery.NewDocumentFromReader(bytes.NewReader(out.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if got := doc.Find(`#tab-panel-orgs > ul > li > ul > li, .ui2-admin > ul > li > ul > li`).Length(); got != 1 {
				t.Errorf("expected one nested organization, got %d", got)
			}
			if got := doc.Find(`section form[action="/admin/orgs"] input[name="_csrf"]`).Length(); got != 1 {
				t.Errorf("expected create form and CSRF in named section, got %d", got)
			}
		})
	}
}
