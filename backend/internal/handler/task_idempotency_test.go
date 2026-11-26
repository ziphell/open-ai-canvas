package handler

import "testing"

func TestResolveIdempotencyKey(t *testing.T) {
	if key, err := resolveIdempotencyKey("", "body-key"); err != nil || key != "body-key" {
		t.Fatalf("body key: %q %v", key, err)
	}
	if key, err := resolveIdempotencyKey("header-key", ""); err != nil || key != "header-key" {
		t.Fatalf("header key: %q %v", key, err)
	}
	if key, err := resolveIdempotencyKey("same-key", "same-key"); err != nil || key != "same-key" {
		t.Fatalf("same key: %q %v", key, err)
	}
	if _, err := resolveIdempotencyKey("header-key", "body-key"); err == nil {
		t.Fatal("mismatched keys should fail")
	}
}
