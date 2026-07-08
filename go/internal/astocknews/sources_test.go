package astocknews

import (
	"path/filepath"
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

func TestUpdateSettingAndFilterRecommendationItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.json")

	if err := UpdateSetting(path, provider.SourceTypeEastMoneyKuaixun, "recommendation", false); err != nil {
		t.Fatalf("UpdateSetting recommendation error: %v", err)
	}
	if err := UpdateSetting(path, provider.SourceTypeEastMoneyFull, "crawl", false); err != nil {
		t.Fatalf("UpdateSetting crawl error: %v", err)
	}
	settings, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if RecommendationEnabled(settings, provider.SourceTypeEastMoneyKuaixun) {
		t.Fatal("expected eastmoney kuaixun recommendation to be disabled")
	}
	if CrawlEnabled(settings, provider.SourceTypeEastMoneyFull) {
		t.Fatal("expected eastmoney full crawl to be disabled")
	}
	filtered := FilterRecommendationItems([]model.Item{
		{SourceType: provider.SourceTypeFlash, Title: "金十新闻"},
		{SourceType: provider.SourceTypeEastMoneyKuaixun, Title: "东方财富快讯"},
		{SourceType: provider.SourceTypeEastMoneyFull, Title: "东方财富全站"},
	}, settings)
	if len(filtered) != 2 || filtered[0].SourceType != provider.SourceTypeFlash || filtered[1].SourceType != provider.SourceTypeEastMoneyFull {
		t.Fatalf("unexpected filtered recommendation items: %+v", filtered)
	}
}
