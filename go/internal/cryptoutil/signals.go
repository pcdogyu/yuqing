package cryptoutil

import (
	"math"
	"strings"
)

type reasonRule struct {
	category string
	label    string
	terms    []string
}

var positiveTerms = []string{
	"上涨", "上扬", "走强", "拉升", "突破", "利好", "增持", "流入", "买入", "反弹", "新高", "获批", "通过",
	"surge", "rise", "rally", "breakout", "bullish", "inflow", "approval", "buy",
}

var negativeTerms = []string{
	"下跌", "回落", "走弱", "利空", "抛售", "流出", "暴跌", "黑客", "漏洞", "清算", "监管打击", "风险", "减持",
	"drop", "fall", "selloff", "bearish", "outflow", "hack", "breach", "liquidation", "risk", "exploit",
}

var reasonRules = []reasonRule{
	{category: "macro", label: "宏观流动性", terms: []string{"美联储", "降息", "通胀", "cpi", "就业", "宏观", "美元指数", "加息", "fed", "inflation", "macro"}},
	{category: "regulation", label: "监管政策", terms: []string{"监管", "合规", "禁令", "执法", "牌照", "法案", "sec", "诉讼", "policy", "regulation"}},
	{category: "exchange", label: "交易所动态", terms: []string{"交易所", "上币", "下架", "binance", "okx", "bybit", "coinbase", "listing", "delist"}},
	{category: "onchain_whale", label: "链上鲸鱼资金", terms: []string{"链上", "巨鲸", "whale", "转账", "地址", "净流入", "净流出"}},
	{category: "institution_etf", label: "机构与 ETF", terms: []string{"etf", "机构", "基金", "资管", "blackrock", "灰度", "grayscale", "institution"}},
	{category: "risk_security", label: "安全与风险事件", terms: []string{"黑客", "漏洞", "攻击", "清算", "爆仓", "诈骗", "风险", "security", "exploit", "liquidation"}},
}

func ScoreDirection(text string) float64 {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return 0
	}
	score := 0.0
	for _, term := range positiveTerms {
		if strings.Contains(lower, strings.ToLower(term)) {
			score += 1.2
		}
	}
	for _, term := range negativeTerms {
		if strings.Contains(lower, strings.ToLower(term)) {
			score -= 1.2
		}
	}
	switch {
	case strings.Contains(lower, "etf"), strings.Contains(lower, "获批"), strings.Contains(lower, "approval"):
		score += 0.8
	case strings.Contains(lower, "hack"), strings.Contains(lower, "黑客"), strings.Contains(lower, "爆仓"):
		score -= 0.8
	}
	return score
}

func DirectionLabel(score float64) string {
	switch {
	case score > 0.35:
		return "bullish"
	case score < -0.35:
		return "bearish"
	default:
		return "neutral"
	}
}

func DetectReason(text string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, rule := range reasonRules {
		for _, term := range rule.terms {
			if strings.Contains(lower, strings.ToLower(term)) {
				return rule.category, rule.label
			}
		}
	}
	return "macro", "市场情绪与流动性"
}

func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}
