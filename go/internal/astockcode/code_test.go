package astockcode

import "testing"

func TestDisplayNameCanonicalAliases(t *testing.T) {
	if got := DisplayName("601881", "银河证券"); got != "中国银河" {
		t.Fatalf("expected 601881 alias to canonicalize to 中国银河, got %q", got)
	}
	if got := DisplayName("601881", "601881 银河证券"); got != "中国银河" {
		t.Fatalf("expected prefixed 601881 alias to canonicalize to 中国银河, got %q", got)
	}
}
