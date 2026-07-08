package astocknews

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

const settingsPathEnv = "YUQING_A_STOCK_NEWS_SOURCE_CONFIG"

type Source struct {
	Type  string
	Label string
	URL   string
}

type SourceSetting struct {
	SourceType            string `json:"source_type"`
	CrawlEnabled          bool   `json:"crawl_enabled"`
	RecommendationEnabled bool   `json:"recommendation_enabled"`
}

type SettingsFile struct {
	Sources []SourceSetting `json:"sources"`
}

func Sources() []Source {
	return []Source{
		{Type: provider.SourceTypeFlash, Label: "金十快讯", URL: "https://www.jin10.com/"},
		{Type: provider.SourceTypeHeadline, Label: "金十资讯", URL: "https://xnews.jin10.com/"},
		{Type: provider.SourceTypeJin10Full, Label: "金十全站", URL: "https://www.jin10.com/"},
		{Type: provider.SourceTypeEastMoneyKuaixun, Label: "东方财富快讯", URL: "https://kuaixun.eastmoney.com/"},
		{Type: provider.SourceTypeEastMoneyFull, Label: "东方财富全站", URL: "https://so.eastmoney.com/"},
		{Type: provider.SourceTypeWallStreetCNAStock, Label: "华尔街见闻", URL: "https://wallstreetcn.com/live/a-stock"},
		{Type: provider.SourceTypeCLSTelegraph, Label: "财联社", URL: "https://www.cls.cn/telegraph"},
		{Type: provider.SourceTypeSinaFinance7x24, Label: "新浪财经", URL: "https://finance.sina.com.cn/7x24/?tag=10"},
	}
}

func SourceTypes() []string {
	sources := Sources()
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		out = append(out, source.Type)
	}
	return out
}

func SourceByType(sourceType string) (Source, bool) {
	canonical := provider.CanonicalSourceType(sourceType)
	for _, source := range Sources() {
		if source.Type == canonical {
			return source, true
		}
	}
	return Source{}, false
}

func Label(sourceType string) string {
	if source, ok := SourceByType(sourceType); ok {
		return source.Label
	}
	sourceType = strings.TrimSpace(sourceType)
	if sourceType == "" {
		return "未知来源"
	}
	return sourceType
}

func URL(sourceType string) string {
	if source, ok := SourceByType(sourceType); ok {
		return source.URL
	}
	return ""
}

func DefaultSettings() map[string]SourceSetting {
	settings := make(map[string]SourceSetting)
	for _, source := range Sources() {
		settings[source.Type] = SourceSetting{
			SourceType:            source.Type,
			CrawlEnabled:          true,
			RecommendationEnabled: true,
		}
	}
	return settings
}

func SettingsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = strings.TrimSpace(os.Getenv(settingsPathEnv))
	}
	if path == "" {
		path = filepath.Join("data", "a-stock-news-sources.json")
	}
	return path
}

func LoadSettings(path string) (map[string]SourceSetting, error) {
	settings := DefaultSettings()
	path = SettingsPath(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return settings, err
	}
	var payload SettingsFile
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, err
	}
	for _, setting := range payload.Sources {
		sourceType := provider.CanonicalSourceType(setting.SourceType)
		if _, ok := settings[sourceType]; !ok {
			continue
		}
		setting.SourceType = sourceType
		settings[sourceType] = setting
	}
	return settings, nil
}

func SaveSettings(path string, settings map[string]SourceSetting) error {
	normalized := DefaultSettings()
	for sourceType, setting := range settings {
		canonical := provider.CanonicalSourceType(sourceType)
		if _, ok := normalized[canonical]; !ok {
			continue
		}
		setting.SourceType = canonical
		normalized[canonical] = setting
	}
	payload := SettingsFile{Sources: make([]SourceSetting, 0, len(normalized))}
	for _, source := range Sources() {
		payload.Sources = append(payload.Sources, normalized[source.Type])
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	path = SettingsPath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func UpdateSetting(path string, sourceType string, field string, enabled bool) error {
	settings, err := LoadSettings(path)
	if err != nil {
		return err
	}
	canonical := provider.CanonicalSourceType(sourceType)
	setting, ok := settings[canonical]
	if !ok {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "crawl", "crawl_enabled":
		setting.CrawlEnabled = enabled
	case "recommend", "recommendation", "recommendation_enabled":
		setting.RecommendationEnabled = enabled
	default:
		return nil
	}
	settings[canonical] = setting
	return SaveSettings(path, settings)
}

func CrawlEnabled(settings map[string]SourceSetting, sourceType string) bool {
	setting, ok := settings[provider.CanonicalSourceType(sourceType)]
	return !ok || setting.CrawlEnabled
}

func RecommendationEnabled(settings map[string]SourceSetting, sourceType string) bool {
	setting, ok := settings[provider.CanonicalSourceType(sourceType)]
	return !ok || setting.RecommendationEnabled
}

func FilterRecommendationItems(items []model.Item, settings map[string]SourceSetting) []model.Item {
	if len(items) == 0 {
		return items
	}
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		if RecommendationEnabled(settings, item.SourceType) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
