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
	foresight := stubProvider{sourceType: SourceTypeForesightNewsflash}
	coindesk := stubProvider{sourceType: SourceTypeCoinDeskZHLatest}
	panews := stubProvider{sourceType: SourceTypePANewsNewsflash}
	theBlock := stubProvider{sourceType: SourceTypeTheBlockLatest}
	eastmoney := stubProvider{sourceType: SourceTypeEastMoneyKuaixun}
	wallstreetcn := stubProvider{sourceType: SourceTypeWallStreetCNAStock}
	cls := stubProvider{sourceType: SourceTypeCLSTelegraph}
	sina := stubProvider{sourceType: SourceTypeSinaFinance7x24}
	registry := Registry{Flash: flash, Headline: headline, Jin10Full: jin10Full, CryptoX: cryptoX, CryptoTelegram: telegram, ForesightNewsflash: foresight, CoinDeskZHLatest: coindesk, PANewsNewsflash: panews, TheBlockLatest: theBlock, EastMoneyKuaixun: eastmoney, WallStreetCNAStock: wallstreetcn, CLSTelegraph: cls, SinaFinance7x24: sina}

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
	if got := registry.Resolve(SourceTypeForesightNewsflash); got != foresight {
		t.Fatalf("expected foresight provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeCoinDeskZHLatest); got != coindesk {
		t.Fatalf("expected coindesk provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypePANewsNewsflash); got != panews {
		t.Fatalf("expected panews provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeTheBlockLatest); got != theBlock {
		t.Fatalf("expected theblock provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeEastMoneyKuaixun); got != eastmoney {
		t.Fatalf("expected eastmoney kuaixun provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeWallStreetCNAStock); got != wallstreetcn {
		t.Fatalf("expected wallstreetcn a-stock provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeCLSTelegraph); got != cls {
		t.Fatalf("expected cls telegraph provider, got %#v", got)
	}
	if got := registry.Resolve(SourceTypeSinaFinance7x24); got != sina {
		t.Fatalf("expected sina finance 7x24 provider, got %#v", got)
	}
	if got := registry.Resolve("unknown"); got != nil {
		t.Fatalf("expected nil for unknown source type, got %#v", got)
	}
}

func TestRegistryAllPreservesSlots(t *testing.T) {
	flash := stubProvider{sourceType: SourceTypeFlash}
	registry := Registry{Flash: flash}

	all := registry.All()
	if len(all) != 13 {
		t.Fatalf("expected 13 providers, got %d", len(all))
	}
	if all[0] != flash {
		t.Fatalf("expected flash provider in first slot, got %#v", all[0])
	}
	if all[1] != nil {
		t.Fatalf("expected nil second slot when headline provider missing, got %#v", all[1])
	}
	for i := 2; i < len(all); i++ {
		if all[i] != nil {
			t.Fatalf("expected nil provider slot %d when provider missing, got %#v", i, all[i])
		}
	}
}
