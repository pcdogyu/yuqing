package cryptoutil

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestResolvePair(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPair     string
		wantDisplay  string
		wantBase     string
		wantQuote    string
		wantBaseName string
		wantBaseCN   string
		wantTerms    []string
	}{
		{
			name:         "separator pair",
			input:        " BTC/USDT ",
			wantPair:     "BTCUSDT",
			wantDisplay:  "BTC/USDT",
			wantBase:     "BTC",
			wantQuote:    "USDT",
			wantBaseName: "Bitcoin",
			wantBaseCN:   "比特币",
			wantTerms:    []string{"BTC", "Bitcoin", "比特币", "BTCUSDT", "BTC/USDT"},
		},
		{
			name:         "compact pair",
			input:        "solusdt",
			wantPair:     "SOLUSDT",
			wantDisplay:  "SOL/USDT",
			wantBase:     "SOL",
			wantQuote:    "USDT",
			wantBaseName: "Solana",
			wantBaseCN:   "索拉纳",
			wantTerms:    []string{"SOL", "Solana", "索拉纳", "SOLUSDT", "SOL/USDT"},
		},
		{
			name:         "alias pair with space separator",
			input:        "大饼 u",
			wantPair:     "BTCUSDT",
			wantDisplay:  "BTC/USDT",
			wantBase:     "BTC",
			wantQuote:    "USDT",
			wantBaseName: "Bitcoin",
			wantBaseCN:   "比特币",
			wantTerms:    []string{"BTC", "Bitcoin", "比特币", "BTCUSDT", "BTC/USDT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePair(tt.input)
			if err != nil {
				t.Fatalf("ResolvePair(%q) returned error: %v", tt.input, err)
			}
			if got.Input != strings.TrimSpace(tt.input) {
				t.Fatalf("ResolvePair(%q) input = %q, want %q", tt.input, got.Input, strings.TrimSpace(tt.input))
			}
			if got.Pair != tt.wantPair {
				t.Fatalf("ResolvePair(%q) pair = %q, want %q", tt.input, got.Pair, tt.wantPair)
			}
			if got.DisplayPair != tt.wantDisplay {
				t.Fatalf("ResolvePair(%q) display = %q, want %q", tt.input, got.DisplayPair, tt.wantDisplay)
			}
			if got.BaseAsset != tt.wantBase {
				t.Fatalf("ResolvePair(%q) base = %q, want %q", tt.input, got.BaseAsset, tt.wantBase)
			}
			if got.QuoteAsset != tt.wantQuote {
				t.Fatalf("ResolvePair(%q) quote = %q, want %q", tt.input, got.QuoteAsset, tt.wantQuote)
			}
			if got.BaseName != tt.wantBaseName {
				t.Fatalf("ResolvePair(%q) base name = %q, want %q", tt.input, got.BaseName, tt.wantBaseName)
			}
			if got.BaseNameCN != tt.wantBaseCN {
				t.Fatalf("ResolvePair(%q) base CN name = %q, want %q", tt.input, got.BaseNameCN, tt.wantBaseCN)
			}
			for _, want := range tt.wantTerms {
				if !slices.Contains(got.SearchTerms, want) {
					t.Fatalf("ResolvePair(%q) search terms = %v, missing %q", tt.input, got.SearchTerms, want)
				}
			}
		})
	}
}

func TestResolvePairUnsupported(t *testing.T) {
	_, err := ResolvePair("abc/xyz")
	if !errors.Is(err, ErrUnsupportedPair) {
		t.Fatalf("ResolvePair returned err = %v, want %v", err, ErrUnsupportedPair)
	}
}

func TestBuildSearchTermsDeduplicatesCaseInsensitiveTerms(t *testing.T) {
	base := assetDef{
		symbol: "BTC",
		name:   "bitcoin",
		nameCN: "Bitcoin",
	}
	quote := assetDef{symbol: "USDT"}

	got := BuildSearchTerms(base, quote)
	want := []string{"BTC", "bitcoin", "BTCUSDT", "BTC/USDT"}
	if !slices.Equal(got, want) {
		t.Fatalf("BuildSearchTerms() = %v, want %v", got, want)
	}
}
