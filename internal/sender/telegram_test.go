package sender

import "testing"

func TestEscapeTelegramHTML(t *testing.T) {
	in := "a&b<c>d"
	got := escapeTelegramHTML(in)
	want := "a&amp;b&lt;c&gt;d"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestSplitByLimit(t *testing.T) {
	text := make([]byte, 0, 9000)
	for i := 0; i < 9000; i++ {
		text = append(text, 'a')
	}

	parts := splitByLimit(string(text), 4000)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	for i, p := range parts {
		if len(p) > 4000 {
			t.Fatalf("part %d too long: %d", i, len(p))
		}
	}
}
