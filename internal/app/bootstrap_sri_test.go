package app

import "testing"

func TestSRI384(t *testing.T) {
	got := sri384([]byte("abc"))
	want := "sha384-ywB1P0WjXou1oD1pmsZQBycsMqsO3tFjGotgWkP/W+2AhgcroefMI1i67KE0yCWn"
	if got != want {
		t.Fatalf("sri384 mismatch: got %q want %q", got, want)
	}
}
