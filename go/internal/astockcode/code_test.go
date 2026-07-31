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

func TestIsInvalidRecommendationNameRejectsNewsPhrases(t *testing.T) {
	for _, name := range []string{"存储芯片大幅高开", "半导体板块异动", "主力资金监控"} {
		if !IsInvalidRecommendationName(name) {
			t.Fatalf("expected news phrase %q to be invalid recommendation name", name)
		}
	}
	for _, name := range []string{"德明利", "中铝国际", "抚顺特钢"} {
		if IsInvalidRecommendationName(name) {
			t.Fatalf("expected stock name %q to be valid recommendation name", name)
		}
	}
}
