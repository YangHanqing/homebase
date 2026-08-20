package ident

import "testing"

func TestValidateWindowIndex(t *testing.T) {
	for _, i := range []int{0, 1, 9999} {
		if err := ValidateWindowIndex(i); err != nil {
			t.Errorf("ValidateWindowIndex(%d) = %v, want nil", i, err)
		}
	}
	for _, i := range []int{-1, 10000} {
		if err := ValidateWindowIndex(i); err == nil {
			t.Errorf("ValidateWindowIndex(%d) = nil, want error", i)
		}
	}
}

func TestValidateName(t *testing.T) {
	if err := ValidateName("  Home Mac mini  "); err != nil {
		t.Fatal(err)
	}
	if err := ValidateName(""); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := ValidateName("bad\nname"); err == nil {
		t.Fatal("control char should fail")
	}
	if err := ValidateName(";"); err == nil {
		t.Fatal("bare semicolon should fail")
	}
	long := make([]rune, 65)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateName(string(long)); err == nil {
		t.Fatal("oversize name should fail")
	}
}
