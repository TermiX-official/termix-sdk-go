package termix

import "testing"

func TestCheckCompat(t *testing.T) {
	var warned []string
	warn := func(m string) { warned = append(warned, m) }

	c := New(Options{Chain: "bsc", OnWarning: warn})
	c.checkCompat(&Contracts{ChainID: 56})
	if len(warned) != 0 {
		t.Fatalf("unexpected warning on match: %v", warned)
	}

	c2 := New(Options{Chain: "bsc", OnWarning: warn})
	c2.checkCompat(&Contracts{ChainID: 97})
	if len(warned) != 1 {
		t.Fatalf("want 1 warning on mismatch, got %d", len(warned))
	}

	c3 := New(Options{BaseURL: "https://x.example/api/v1", OnWarning: warn})
	c3.checkCompat(&Contracts{ChainID: 97})
	if len(warned) != 1 {
		t.Fatalf("baseURL override should skip the check; warnings=%d", len(warned))
	}
}
