package cookieutil

import "testing"

func TestFilterCookieHeader_NoAllowlist(t *testing.T) {
	in := "a=1; b=2"
	got, applied := FilterCookieHeader(in, "")
	if applied {
		t.Fatalf("applied = true, want false")
	}
	if got != in {
		t.Fatalf("got %q, want %q", got, in)
	}
}

func TestFilterCookieHeader_ExactMatch(t *testing.T) {
	in := "a=1; b=2; c=3"
	got, applied := FilterCookieHeader(in, "b,c")
	if !applied {
		t.Fatalf("applied = false, want true")
	}
	if got != "b=2; c=3" {
		t.Fatalf("got %q, want %q", got, "b=2; c=3")
	}
}

func TestFilterCookieHeader_PrefixMatch(t *testing.T) {
	in := "__Secure-1PSID=aaa; __Secure-3PSID=bbb; NID=ccc"
	got, _ := FilterCookieHeader(in, "__Secure-*")
	if got != "__Secure-1PSID=aaa; __Secure-3PSID=bbb" {
		t.Fatalf("got %q, want %q", got, "__Secure-1PSID=aaa; __Secure-3PSID=bbb")
	}
}

func TestFilterCookieHeader_TrimsAndIgnoresUnknownParts(t *testing.T) {
	in := " a=1 ;; b=2; ; c=3 "
	got, _ := FilterCookieHeader(in, "a,c")
	if got != "a=1; c=3" {
		t.Fatalf("got %q, want %q", got, "a=1; c=3")
	}
}
