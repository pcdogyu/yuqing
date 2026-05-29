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
	registry := Registry{Flash: flash, Headline: headline}

	if got := registry.Resolve(SourceTypeFlash); got != flash {
		t.Fatalf("expected flash provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeHeadline); got != headline {
		t.Fatalf("expected headline provider, got %#v", got)
	}
	if got := registry.Resolve("unknown"); got != nil {
		t.Fatalf("expected nil for unknown source type, got %#v", got)
	}
}

func TestRegistryAllPreservesSlots(t *testing.T) {
	flash := stubProvider{sourceType: SourceTypeFlash}
	registry := Registry{Flash: flash}

	all := registry.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(all))
	}
	if all[0] != flash {
		t.Fatalf("expected flash provider in first slot, got %#v", all[0])
	}
	if all[1] != nil {
		t.Fatalf("expected nil second slot when headline provider missing, got %#v", all[1])
	}
}
