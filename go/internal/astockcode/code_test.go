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

func TestHasResolvedNameRejectsGarbledNames(t *testing.T) {
	for _, name := range []string{"????", "中国�星"} {
		if HasResolvedName("600118", name) {
			t.Fatalf("expected garbled name %q to be unresolved", name)
		}
	}
	if !HasResolvedName("600118", "中国卫星") {
		t.Fatal("expected normal stock name to be resolved")
	}
}
