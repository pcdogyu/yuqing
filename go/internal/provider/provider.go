package provider

import (
	"context"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	SourceTypeFlash              = "jin10_kuaixun"
	SourceTypeHeadline           = "jin10_资讯"
	LegacySourceTypeFlash        = "flash"
	LegacySourceTypeHeadline     = "headline"
	SourceTypeJin10Full          = "jin10_full"
	SourceTypeCryptoX            = "crypto_x"
	SourceTypeCryptoTelegram     = "crypto_telegram"
	SourceTypeForesightNewsflash = "foresight_newsflash"
	SourceTypeCoinDeskZHLatest   = "coindesk_zh_latest"
	SourceTypePANewsNewsflash    = "panews_newsflash"
	SourceTypeTheBlockLatest     = "theblock_latest"
	SourceTypeEastMoneyKuaixun   = "eastmoney_kuaixun"
	SourceTypeEastMoneyFull      = "eastmoney_full"
	SourceTypeWallStreetCNAStock = "wallstreetcn_a_stock"
	SourceTypeCLSTelegraph       = "cls_telegraph"
	SourceTypeSinaFinance7x24    = "sina_finance_7x24"
)

func ValidSourceType(value string) string {
	switch strings.TrimSpace(value) {
	case SourceTypeFlash, LegacySourceTypeFlash:
		return SourceTypeFlash
	case SourceTypeHeadline, LegacySourceTypeHeadline:
		return SourceTypeHeadline
	case SourceTypeJin10Full:
		return SourceTypeJin10Full
	case SourceTypeCryptoX:
		return SourceTypeCryptoX
	case SourceTypeCryptoTelegram:
		return SourceTypeCryptoTelegram
	case SourceTypeForesightNewsflash:
		return SourceTypeForesightNewsflash
	case SourceTypeCoinDeskZHLatest:
		return SourceTypeCoinDeskZHLatest
	case SourceTypePANewsNewsflash:
		return SourceTypePANewsNewsflash
	case SourceTypeTheBlockLatest:
		return SourceTypeTheBlockLatest
	case SourceTypeEastMoneyKuaixun:
		return SourceTypeEastMoneyKuaixun
	case SourceTypeEastMoneyFull:
		return SourceTypeEastMoneyFull
	case SourceTypeWallStreetCNAStock:
		return SourceTypeWallStreetCNAStock
	case SourceTypeCLSTelegraph:
		return SourceTypeCLSTelegraph
	case SourceTypeSinaFinance7x24:
		return SourceTypeSinaFinance7x24
	default:
		return ""
	}
}

func SourceTypeAliases(value string) []string {
	switch ValidSourceType(value) {
	case SourceTypeFlash:
		return []string{SourceTypeFlash, LegacySourceTypeFlash}
	case SourceTypeHeadline:
		return []string{SourceTypeHeadline, LegacySourceTypeHeadline}
	case "":
		return nil
	default:
		return []string{ValidSourceType(value)}
	}
}

func CanonicalSourceType(value string) string {
	canonical := ValidSourceType(value)
	if canonical != "" {
		return canonical
	}
	return strings.TrimSpace(value)
}

type Provider interface {
	SourceType() string
	Fetch(context.Context) ([]model.Item, error)
}

type WindowedProvider interface {
	Provider
	FetchWithOptions(context.Context, model.CrawlOptions) ([]model.Item, error)
}

type Registry struct {
	Flash              Provider
	Headline           Provider
	Jin10Full          Provider
	CryptoX            Provider
	CryptoTelegram     Provider
	ForesightNewsflash Provider
	CoinDeskZHLatest   Provider
	PANewsNewsflash    Provider
	TheBlockLatest     Provider
	EastMoneyKuaixun   Provider
	EastMoneyFull      Provider
	WallStreetCNAStock Provider
	CLSTelegraph       Provider
	SinaFinance7x24    Provider
}

func (r Registry) Resolve(sourceType string) Provider {
	switch ValidSourceType(sourceType) {
	case SourceTypeFlash:
		return r.Flash
	case SourceTypeHeadline:
		return r.Headline
	case SourceTypeJin10Full:
		return r.Jin10Full
	case SourceTypeCryptoX:
		return r.CryptoX
	case SourceTypeCryptoTelegram:
		return r.CryptoTelegram
	case SourceTypeForesightNewsflash:
		return r.ForesightNewsflash
	case SourceTypeCoinDeskZHLatest:
		return r.CoinDeskZHLatest
	case SourceTypePANewsNewsflash:
		return r.PANewsNewsflash
	case SourceTypeTheBlockLatest:
		return r.TheBlockLatest
	case SourceTypeEastMoneyKuaixun:
		return r.EastMoneyKuaixun
	case SourceTypeEastMoneyFull:
		return r.EastMoneyFull
	case SourceTypeWallStreetCNAStock:
		return r.WallStreetCNAStock
	case SourceTypeCLSTelegraph:
		return r.CLSTelegraph
	case SourceTypeSinaFinance7x24:
		return r.SinaFinance7x24
	default:
		return nil
	}
}

func (r Registry) All() []Provider {
	return []Provider{r.Flash, r.Headline, r.Jin10Full, r.CryptoX, r.CryptoTelegram, r.ForesightNewsflash, r.CoinDeskZHLatest, r.PANewsNewsflash, r.TheBlockLatest, r.EastMoneyKuaixun, r.EastMoneyFull, r.WallStreetCNAStock, r.CLSTelegraph, r.SinaFinance7x24}
}
