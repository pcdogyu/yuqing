package provider

import (
	"context"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	SourceTypeFlash              = "flash"
	SourceTypeHeadline           = "headline"
	SourceTypeJin10Full          = "jin10_full"
	SourceTypeCryptoX            = "crypto_x"
	SourceTypeCryptoTelegram     = "crypto_telegram"
	SourceTypeForesightNewsflash = "foresight_newsflash"
	SourceTypeCoinDeskZHLatest   = "coindesk_zh_latest"
	SourceTypePANewsNewsflash    = "panews_newsflash"
	SourceTypeTheBlockLatest     = "theblock_latest"
	SourceTypeEastMoneyKuaixun   = "eastmoney_kuaixun"
)

func ValidSourceType(value string) string {
	switch strings.TrimSpace(value) {
	case SourceTypeFlash:
		return SourceTypeFlash
	case SourceTypeHeadline:
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
	default:
		return ""
	}
}

type Provider interface {
	SourceType() string
	Fetch(context.Context) ([]model.Item, error)
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
}

func (r Registry) Resolve(sourceType string) Provider {
	switch sourceType {
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
	default:
		return nil
	}
}

func (r Registry) All() []Provider {
	return []Provider{r.Flash, r.Headline, r.Jin10Full, r.CryptoX, r.CryptoTelegram, r.ForesightNewsflash, r.CoinDeskZHLatest, r.PANewsNewsflash, r.TheBlockLatest, r.EastMoneyKuaixun}
}
