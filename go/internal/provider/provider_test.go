package provider

import (
	"context"
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type stubProvider struct {
	sourceType string
}

func (s stubProvider) SourceType() string {
	return s.sourceType
}

func (s stubProvider) Fetch(context.Context) ([]model.Item, error) {
	return nil, nil
}

func TestRegistryResolve(t *testing.T) {
	flash := stubProvider{sourceType: SourceTypeFlash}
	headline := stubProvider{sourceType: SourceTypeHeadline}
	cryptoX := stubProvider{sourceType: SourceTypeCryptoX}
	telegram := stubProvider{sourceType: SourceTypeCryptoTelegram}
	registry := Registry{Flash: flash, Headline: headline, CryptoX: cryptoX, CryptoTelegram: telegram}

	if got := registry.Resolve(SourceTypeFlash); got != flash {
		t.Fatalf("expected flash provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeHeadline); got != headline {
		t.Fatalf("expected headline provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeCryptoX); got != cryptoX {
		t.Fatalf("expected crypto x provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeCryptoTelegram); got != telegram {
		t.Fatalf("expected crypto telegram provider, got %#v", got)
	}
	if got := registry.Resolve("unknown"); got != nil {
		t.Fatalf("expected nil for unknown source type, got %#v", got)
	}
}

func TestRegistryAllPreservesSlots(t *testing.T) {
	flash := stubProvider{sourceType: SourceTypeFlash}
	registry := Registry{Flash: flash}

	all := registry.All()
	if len(all) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(all))
	}
	if all[0] != flash {
		t.Fatalf("expected flash provider in first slot, got %#v", all[0])
	}
	if all[1] != nil {
		t.Fatalf("expected nil second slot when headline provider missing, got %#v", all[1])
	}
	if all[2] != nil || all[3] != nil {
		t.Fatalf("expected nil social slots when providers missing, got %#v %#v", all[2], all[3])
	}
}
