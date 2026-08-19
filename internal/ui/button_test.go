package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/3lbits/vigil/internal/csrf"
)

var (
	allBtnVariants = []BtnVariant{BtnPrimary, BtnSecondary, BtnTertiary}
	allBtnColors   = []BtnColor{BtnAccent, BtnDanger, BtnSuccess}
)

func TestBtnStylesComplete(t *testing.T) {
	want := len(allBtnVariants) * len(allBtnColors)
	if len(btnStyles) != want {
		t.Errorf("btnStyles has %d entries, want %d", len(btnStyles), want)
	}
	for _, v := range allBtnVariants {
		for _, c := range allBtnColors {
			if btnStyles[btnKey{v, c}] == "" {
				t.Errorf("no styles for {%q, %q}", v, c)
			}
		}
	}
}

func TestBtnStylesDistinct(t *testing.T) {
	seen := map[string]btnKey{}
	for _, v := range allBtnVariants {
		for _, c := range allBtnColors {
			k := btnKey{v, c}
			s := btnStyles[k]
			if prev, dup := seen[s]; dup {
				t.Errorf("%v and %v share the same styles", prev, k)
			}
			seen[s] = k
		}
	}
}

func TestBtnFocusRingMatchesColor(t *testing.T) {
	wantRing := map[BtnColor]string{
		BtnAccent:  "focus:ring-blue-",
		BtnDanger:  "focus:ring-red-",
		BtnSuccess: "focus:ring-green-",
	}
	for _, v := range allBtnVariants {
		for _, c := range allBtnColors {
			if !strings.Contains(btnStyles[btnKey{v, c}], wantRing[c]) {
				t.Errorf("{%q, %q} missing %s ring", v, c, wantRing[c])
			}
		}
	}
}

func TestBtnRenders(t *testing.T) {
	got := render(t, Btn(BtnProps{Variant: BtnSecondary, Color: BtnDanger}, "Delete"))

	if !strings.Contains(got, `type="submit"`) {
		t.Errorf("missing type=submit: %s", got)
	}
	if !strings.Contains(got, ">Delete<") {
		t.Errorf("missing text: %s", got)
	}
	if !strings.Contains(got, btnStyles[btnKey{BtnSecondary, BtnDanger}]) {
		t.Errorf("wrong styles: %s", got)
	}
}

func TestBtnDisabled(t *testing.T) {
	got := render(t, Btn(BtnProps{Disabled: true}, "Save"))
	if !strings.Contains(got, "disabled") {
		t.Errorf("not disabled: %s", got)
	}
}

func TestBtnOmitsEmptyAriaLabel(t *testing.T) {
	got := render(t, Btn(BtnProps{}, "Save"))
	if strings.Contains(got, "aria-label") {
		t.Errorf("emitted aria-label when Label was empty: %s", got)
	}
}

func TestBtnIconIsHidden(t *testing.T) {
	got := render(t, Btn(BtnProps{Icon: "plus"}, "Add activity"))
	if !strings.Contains(got, `aria-hidden="true"`) {
		t.Errorf("icon not aria-hidden: %s", got)
	}
}

func TestBtnActionConfirm(t *testing.T) {
	with := render(t, BtnAction(ActionProps{Action: "/x", Confirm: "Sure?"}, "Delete"))
	if !strings.Contains(with, `data-confirm="Sure?"`) {
		t.Errorf("missing data-confirm: %s", with)
	}

	without := render(t, BtnAction(ActionProps{Action: "/x"}, "Save"))
	if strings.Contains(without, "data-confirm") {
		t.Errorf("emitted empty data-confirm: %s", without)
	}
}

func TestBtnActionIncludesCSRF(t *testing.T) {
	got := renderCtx(t, csrfCtx(t, "tok-123"),
		BtnAction(ActionProps{Action: "/activities/1/start"}, "Start"))

	if !strings.Contains(got, `name="_csrf" value="tok-123"`) {
		t.Errorf("CSRF token not rendered: %s", got)
	}
	if !strings.Contains(got, `method="POST"`) {
		t.Errorf("missing method: %s", got)
	}
}

func csrfCtx(t *testing.T, token string) context.Context {
	t.Helper()
	return csrf.WithToken(context.Background(), token)
}
