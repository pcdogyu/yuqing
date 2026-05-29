package provider

import (
	"context"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

const (
	SourceTypeFlash    = "flash"
	SourceTypeHeadline = "headline"
)

type Provider interface {
	SourceType() string
	Fetch(context.Context) ([]model.Item, error)
}

type Registry struct {
	Flash    Provider
	Headline Provider
}

func (r Registry) Resolve(sourceType string) Provider {
	switch sourceType {
	case SourceTypeFlash:
		return r.Flash
	case SourceTypeHeadline:
		return r.Headline
	default:
		return nil
	}
}

func (r Registry) All() []Provider {
	return []Provider{r.Flash, r.Headline}
}
