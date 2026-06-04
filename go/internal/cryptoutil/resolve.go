package cryptoutil

import (
	"errors"
	"sort"
	"strings"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

var ErrUnsupportedPair = errors.New("unsupported crypto pair")

type assetDef struct {
	symbol   string
	name     string
	nameCN   string
	aliases  []string
	priority int
}

var baseAssets = []assetDef{
	{symbol: "BTC", name: "Bitcoin", nameCN: "比特币", aliases: []string{"BTC", "BITCOIN", "XBT", "比特币", "大饼"}, priority: 10},
	{symbol: "ETH", name: "Ethereum", nameCN: "以太坊", aliases: []string{"ETH", "ETHEREUM", "以太坊", "姨太"}, priority: 9},
	{symbol: "SOL", name: "Solana", nameCN: "索拉纳", aliases: []string{"SOL", "SOLANA", "索拉纳"}, priority: 8},
	{symbol: "BNB", name: "BNB", nameCN: "币安币", aliases: []string{"BNB", "BINANCECOIN", "币安币"}, priority: 7},
	{symbol: "XRP", name: "XRP", nameCN: "瑞波币", aliases: []string{"XRP", "RIPPLE", "瑞波", "瑞波币"}, priority: 6},
	{symbol: "DOGE", name: "Dogecoin", nameCN: "狗狗币", aliases: []string{"DOGE", "DOGECOIN", "狗狗币"}, priority: 5},
}

var quoteAssets = []assetDef{
	{symbol: "USDT", name: "Tether", nameCN: "泰达币", aliases: []string{"USDT", "USD", "U", "泰达", "泰达币"}, priority: 10},
	{symbol: "USDC", name: "USD Coin", nameCN: "美元稳定币", aliases: []string{"USDC"}, priority: 9},
	{symbol: "FDUSD", name: "First Digital USD", nameCN: "FDUSD", aliases: []string{"FDUSD"}, priority: 8},
	{symbol: "BUSD", name: "Binance USD", nameCN: "BUSD", aliases: []string{"BUSD"}, priority: 7},
	{symbol: "BTC", name: "Bitcoin", nameCN: "比特币", aliases: []string{"BTC", "BITCOIN", "比特币"}, priority: 2},
	{symbol: "ETH", name: "Ethereum", nameCN: "以太坊", aliases: []string{"ETH", "ETHEREUM", "以太坊"}, priority: 1},
}

var separators = []string{"/", "-", "_", " "}

func ResolvePair(input string) (model.CryptoPairResolution, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return model.CryptoPairResolution{}, ErrUnsupportedPair
	}
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	for _, sep := range separators {
		if strings.Contains(normalized, sep) {
			parts := strings.Split(normalized, sep)
			if len(parts) < 2 {
				break
			}
			base, okBase := resolveAsset(parts[0], baseAssets)
			quote, okQuote := resolveAsset(parts[1], quoteAssets)
			if !okBase || !okQuote {
				return model.CryptoPairResolution{}, ErrUnsupportedPair
			}
			return buildResolution(raw, base, quote), nil
		}
	}

	compact := strings.NewReplacer("/", "", "-", "", "_", "", " ", "").Replace(normalized)
	quotes := append([]assetDef(nil), quoteAssets...)
	sort.Slice(quotes, func(i, j int) bool {
		if len(quotes[i].symbol) == len(quotes[j].symbol) {
			return quotes[i].priority > quotes[j].priority
		}
		return len(quotes[i].symbol) > len(quotes[j].symbol)
	})
	for _, quote := range quotes {
		if !strings.HasSuffix(compact, quote.symbol) {
			continue
		}
		basePart := strings.TrimSuffix(compact, quote.symbol)
		base, ok := resolveAsset(basePart, baseAssets)
		if !ok {
			continue
		}
		return buildResolution(raw, base, quote), nil
	}
	return model.CryptoPairResolution{}, ErrUnsupportedPair
}

func buildResolution(input string, base, quote assetDef) model.CryptoPairResolution {
	pair := base.symbol + quote.symbol
	displayQuote := quote.symbol
	if displayQuote == "USDT" {
		displayQuote = "USDT"
	}
	return model.CryptoPairResolution{
		Input:         strings.TrimSpace(input),
		Pair:          pair,
		DisplayPair:   base.symbol + "/" + displayQuote,
		BaseAsset:     base.symbol,
		QuoteAsset:    quote.symbol,
		BaseName:      base.name,
		BaseNameCN:    base.nameCN,
		QuoteName:     quote.name,
		BinanceSymbol: pair,
		SearchTerms:   BuildSearchTerms(base, quote),
	}
}

func BuildSearchTerms(base, quote assetDef) []string {
	terms := []string{
		base.symbol,
		base.name,
		base.nameCN,
		base.symbol + quote.symbol,
		base.symbol + "/" + quote.symbol,
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key := strings.ToUpper(term)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, term)
	}
	return result
}

func resolveAsset(raw string, defs []assetDef) (assetDef, bool) {
	target := strings.ToUpper(strings.TrimSpace(raw))
	target = strings.NewReplacer("/", "", "-", "", "_", "", " ", "").Replace(target)
	for _, def := range defs {
		if target == def.symbol {
			return def, true
		}
		for _, alias := range def.aliases {
			normalized := strings.ToUpper(strings.TrimSpace(alias))
			normalized = strings.NewReplacer("/", "", "-", "", "_", "", " ", "").Replace(normalized)
			if target == normalized {
				return def, true
			}
		}
	}
	return assetDef{}, false
}
