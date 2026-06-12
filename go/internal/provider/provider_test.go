package provider

import (
	"context"
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/model"
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
	jin10Full := stubProvider{sourceType: SourceTypeJin10Full}
	cryptoX := stubProvider{sourceType: SourceTypeCryptoX}
	telegram := stubProvider{sourceType: SourceTypeCryptoTelegram}
	registry := Registry{Flash: flash, Headline: headline, Jin10Full: jin10Full, CryptoX: cryptoX, CryptoTelegram: telegram}

	if got := registry.Resolve(SourceTypeFlash); got != flash {
		t.Fatalf("expected flash provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeHeadline); got != headline {
		t.Fatalf("expected headline provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeJin10Full); got != jin10Full {
		t.Fatalf("expected jin10 full provider, got %#v", got)
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
	if len(all) != 5 {
		t.Fatalf("expected 5 providers, got %d", len(all))
	}
	if all[0] != flash {
		t.Fatalf("expected flash provider in first slot, got %#v", all[0])
	}
	if all[1] != nil {
		t.Fatalf("expected nil second slot when headline provider missing, got %#v", all[1])
	}
	if all[2] != nil || all[3] != nil || all[4] != nil {
		t.Fatalf("expected nil full/social slots when providers missing, got %#v %#v %#v", all[2], all[3], all[4])
	}
}
