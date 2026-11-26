package auth

import "testing"

func TestChallengeMatchesS256(t *testing.T) {
	// RFC 7636 appendix B example.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := Challenge(verifier); got != want {
		t.Fatalf("Challenge = %s, want %s", got, want)
	}
}

func TestRandomURLSafe(t *testing.T) {
	first, err := RandomURLSafe(32)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RandomURLSafe(32)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("random values should differ")
	}
	if len(first) != 43 {
		t.Fatalf("unexpected length %d", len(first))
	}
}
