package avviktemplates

import (
	"bytes"
	"context"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestClosureAndTabControls(t *testing.T) {
	for _, canClose := range []bool{false, true} {
		var out bytes.Buffer
		if err := closureTab(DetailData{CanManage: true, CanClose: canClose}).Render(context.Background(), &out); err != nil {
			t.Fatal(err)
		}
		doc, err := goquery.NewDocumentFromReader(&out)
		if err != nil {
			t.Fatal(err)
		}
		if _, disabled := doc.Find(`button[data-tone="danger"]`).Attr("disabled"); disabled == canClose {
			t.Errorf("canClose=%t: wrong disabled state", canClose)
		}
	}

	var out bytes.Buffer
	if err := tabLink("123", "details", "details").Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(&out)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Find(`a[href="/avvik/123?tab=details"][hx-get="/avvik/123?tab=details"].ops-tab-active`).Length() != 1 {
		t.Error("active tab lost its HTMX target or URL")
	}
}
