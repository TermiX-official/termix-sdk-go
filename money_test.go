package termix

import "testing"

func TestMoneyRoundTrip(t *testing.T) {
	for _, v := range []string{"0", "1", "1.5", "24.28", "1000000", "0.000001"} {
		b, err := ToBaseUnits(v, 18)
		if err != nil {
			t.Fatalf("ToBaseUnits(%q): %v", v, err)
		}
		got := FromBaseUnits(b, 18)
		if got != v {
			t.Errorf("round-trip %q -> %q", v, got)
		}
	}
}

func TestMoneyExact(t *testing.T) {
	b, _ := ToBaseUnits("24.28", 18)
	if b.String() != "24280000000000000000" {
		t.Errorf("24.28@18 = %s", b.String())
	}
	b2, _ := ToBaseUnits("0.1", 6)
	if b2.String() != "100000" {
		t.Errorf("0.1@6 = %s", b2.String())
	}
}

func TestMoneyOverPrecision(t *testing.T) {
	if _, err := ToBaseUnits("1.1234567", 6); err == nil {
		t.Error("expected error for over-precise amount")
	}
}

func TestMoneyNegative(t *testing.T) {
	b, _ := ToBaseUnits("-1.5", 18)
	if FromBaseUnits(b, 18) != "-1.5" {
		t.Error("negative round-trip failed")
	}
}
