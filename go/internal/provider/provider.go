package provider

import (
	"context"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	SourceTypeFlash          = "flash"
	SourceTypeHeadline       = "headline"
	SourceTypeJin10Full      = "jin10_full"
	SourceTypeCryptoX        = "crypto_x"
	SourceTypeCryptoTelegram = "crypto_telegram"
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
	default:
		return ""
	}
}

type Provider interface {
	SourceType() string
	Fetch(context.Context) ([]model.Item, error)
}

type Registry struct {
	Flash          Provider
	Headline       Provider
	Jin10Full      Provider
	CryptoX        Provider
	CryptoTelegram Provider
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
	default:
		return nil
	}
}

func (r Registry) All() []Provider {
	return []Provider{r.Flash, r.Headline, r.Jin10Full, r.CryptoX, r.CryptoTelegram}
}
