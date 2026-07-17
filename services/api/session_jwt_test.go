package main

import (
	"testing"
	"time"
)

func TestSessionJWTRoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET", "unit-test-secret-at-least-32-chars!!")
	id := Identity{UserID: "u1", WorkspaceID: "w1", Role: "owner"}
	token, expires, err := issueSessionJWT(id)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(expires) < 50*time.Minute {
		t.Fatalf("unexpected expiry %v", expires)
	}
	got, err := parseSessionJWT(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("got %+v want %+v", got, id)
	}
}

func TestSessionJWTRejectsTamper(t *testing.T) {
	t.Setenv("JWT_SECRET", "unit-test-secret-at-least-32-chars!!")
	token, _, err := issueSessionJWT(Identity{UserID: "u1", WorkspaceID: "w1", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	tampered := token[:len(token)-2] + "aa"
	if _, err := parseSessionJWT(tampered); err == nil {
		t.Fatal("expected tamper rejection")
	}
}

func TestExtractSessionTokenPrefersBearer(t *testing.T) {
	got := extractSessionToken("Bearer abc", "cookie")
	if got != "abc" {
		t.Fatalf("got %q", got)
	}
	got = extractSessionToken("", "cookie-val")
	if got != "cookie-val" {
		t.Fatalf("got %q", got)
	}
}
