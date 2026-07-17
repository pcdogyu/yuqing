package portal

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type aStockAlgorithmParamGroupView struct {
	Key    string
	Label  string
	Params []aStockAlgorithmParamView
}

type aStockAlgorithmParamView struct {
	Key          string
	Label        string
	Value        string
	DefaultValue string
	Unit         string
	InputType    string
	Step         string
	Description  string
}

type aStockAlgorithmParamDef struct {
	Group       string
	GroupLabel  string
	Key         string
	Label       string
	Unit        string
	InputType   string
	Step        string
	Description string
}

func defaultAStockAlgorithmSettings() model.AStockRecommendationAlgorithmSettings {
	return model.DefaultAStockRecommendationAlgorithmSettings()
}

func (s *Server) loadAStockAlgorithmSettings() model.AStockRecommendationAlgorithmSettings {
	settings := defaultAStockAlgorithmSettings()
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/a-stock-recommendation-algorithm", &settings)
	return model.NormalizeAStockRecommendationAlgorithmSettings(settings)
}

func (s *Server) loadAStockAlgorithmSettingsWithCache(cache *aStockRequestCache) model.AStockRecommendationAlgorithmSettings {
	if cache != nil && cache.algorithmSettingsLoaded {
		return cache.algorithmSettings
	}
	settings := s.loadAStockAlgorithmSettings()
	if cache != nil {
		cache.algorithmSettings = settings
		cache.algorithmSettingsLoaded = true
	}
	return settings
}

func buildAStockAlgorithmParamGroups(settings model.AStockRecommendationAlgorithmSettings) []aStockAlgorithmParamGroupView {
	defaults := defaultAStockAlgorithmSettings()
	groups := make([]aStockAlgorithmParamGroupView, 0, 6)
	groupIndex := map[string]int{}
	for _, def := range aStockAlgorithmParamDefs() {
		idx, ok := groupIndex[def.Group]
		if !ok {
			idx = len(groups)
			groupIndex[def.Group] = idx
			groups = append(groups, aStockAlgorithmParamGroupView{Key: def.Group, Label: def.GroupLabel})
		}
		groups[idx].Params = append(groups[idx].Params, aStockAlgorithmParamView{
			Key:          def.Key,
			Label:        def.Label,
			Value:        aStockAlgorithmFieldString(settings, def.Key),
			DefaultValue: aStockAlgorithmFieldString(defaults, def.Key),
			Unit:         def.Unit,
			InputType:    nonEmpty(def.InputType, "number"),
			Step:         nonEmpty(def.Step, "1"),
			Description:  def.Description,
		})
	}
	return groups
}

func decodeAStockAlgorithmSettingsForm(values map[string][]string) (model.AStockRecommendationAlgorithmSettings, error) {
	settings := defaultAStockAlgorithmSettings()
	for _, def := range aStockAlgorithmParamDefs() {
		raw := ""
		if values != nil {
			raw = strings.TrimSpace(firstFormString(values["param."+def.Key]))
		}
		if err := setAStockAlgorithmFieldString(&settings, def.Key, raw); err != nil {
			return model.AStockRecommendationAlgorithmSettings{}, fmt.Errorf("%s: %w", def.Label, err)
		}
	}
	if err := model.ValidateAStockRecommendationAlgorithmSettings(settings); err != nil {
		return model.AStockRecommendationAlgorithmSettings{}, err
	}
	return settings, nil
}

func firstFormString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func aStockAlgorithmParamDefs() []aStockAlgorithmParamDef {
	return []aStockAlgorithmParamDef{
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.factor_score_cap", Label: "因子封顶", Unit: "分", Description: "竞价因子正向最高分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.recommendation_limit", Label: "推荐总数上限", Unit: "个", Description: "最终推荐股票数量上限"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.replacement_pool_limit", Label: "递补候选池", Unit: "个", Description: "资金过滤后用于递补的候选池数量"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.replacement_per_hotspot", Label: "单热点递补数", Unit: "个", Description: "每个热点最多保留的递补候选"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.hotspot_top_stock_limit", Label: "热点股票展示数", Unit: "个", Description: "热点中展示的集合竞价代表股票数量"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.hotspot_scored_candidate_limit", Label: "竞价评分候选数", Unit: "个", Description: "参与热点股票评分的竞价候选数量"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.hotspot_limit", Label: "热点数量上限", Unit: "个", Description: "参与推荐的热点数量"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.market_candidate_limit", Label: "竞价候选总数", Unit: "个", Description: "集合竞价候选池拉取上限"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.market_rank_score_base", Label: "竞价排名基础分", Unit: "分", Description: "行情排名分的基础值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.market_rank_score_divisor", Label: "竞价排名分除数", Unit: "", Description: "排名分计算除数，越小排名影响越大"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.stocks_per_hotspot", Label: "单热点推荐数", Unit: "个", Description: "每个热点最多选出的股票数"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_threshold_1_pct", Label: "高开一档阈值", Unit: "%", Step: "0.01", Description: "高开加分第一档阈值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_score_1", Label: "高开一档加分", Unit: "分", Description: "达到一档高开阈值的加分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_threshold_2_pct", Label: "高开二档阈值", Unit: "%", Step: "0.01", Description: "高开加分第二档阈值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_score_2", Label: "高开二档加分", Unit: "分", Description: "达到二档高开阈值的加分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_threshold_3_pct", Label: "高开三档阈值", Unit: "%", Step: "0.01", Description: "高开加分第三档阈值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_score_3", Label: "高开三档加分", Unit: "分", Description: "达到三档高开阈值的加分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_threshold_4_pct", Label: "高开四档阈值", Unit: "%", Step: "0.01", Description: "高开加分第四档阈值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_score_4", Label: "高开四档加分", Unit: "分", Description: "达到四档高开阈值的加分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_threshold_5_pct", Label: "高开五档阈值", Unit: "%", Step: "0.01", Description: "高开加分第五档阈值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_score_5", Label: "高开五档加分", Unit: "分", Description: "达到五档高开阈值的加分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_strong_threshold_pct", Label: "强高开阈值", Unit: "%", Step: "0.01", Description: "强高开加分阈值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.high_open_strong_score", Label: "强高开加分", Unit: "分", Description: "达到强高开阈值的加分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_threshold_1_pct", Label: "低开一档阈值", Unit: "%", Step: "0.01", Description: "低开幅度达到一档阈值时扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_penalty_1", Label: "低开一档扣分", Unit: "分", Description: "达到一档低开阈值的扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_threshold_2_pct", Label: "低开二档阈值", Unit: "%", Step: "0.01", Description: "低开幅度达到二档阈值时扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_penalty_2", Label: "低开二档扣分", Unit: "分", Description: "达到二档低开阈值的扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_threshold_3_pct", Label: "低开三档阈值", Unit: "%", Step: "0.01", Description: "低开幅度达到三档阈值时扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_penalty_3", Label: "低开三档扣分", Unit: "分", Description: "达到三档低开阈值的扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_threshold_4_pct", Label: "低开四档阈值", Unit: "%", Step: "0.01", Description: "低开幅度达到四档阈值时扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_penalty_4", Label: "低开四档扣分", Unit: "分", Description: "达到四档低开阈值的扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_threshold_5_pct", Label: "低开五档阈值", Unit: "%", Step: "0.01", Description: "低开幅度达到五档阈值时扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_penalty_5", Label: "低开五档扣分", Unit: "分", Description: "达到五档低开阈值的扣分"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_strong_threshold_pct", Label: "强低开阈值", Unit: "%", Step: "0.01", Description: "强低开扣分阈值"},
		{Group: "auction", GroupLabel: "竞价因子", Key: "auction.low_open_strong_penalty", Label: "强低开扣分", Unit: "分", Description: "达到强低开阈值的扣分"},

		{Group: "candidate", GroupLabel: "候选生成", Key: "candidate.require_hotspot_link", Label: "要求热点关联", InputType: "text", Description: "true 时纯集合竞价股票不得直接入池"},
		{Group: "candidate", GroupLabel: "候选生成", Key: "candidate.auction_fallback_per_hotspot", Label: "单热点竞价确认数", Unit: "个", Description: "每个热点最多给多少板块候选附加集合竞价确认"},
		{Group: "candidate", GroupLabel: "候选生成", Key: "candidate.sector_candidate_limit_per_hotspot", Label: "单热点板块候选数", Unit: "个", Description: "每个热点最多收集的板块成分股候选"},
		{Group: "candidate", GroupLabel: "候选生成", Key: "candidate.stock_fund_flow_candidate_limit", Label: "个股资金候选数", Unit: "个", Description: "从5日/10日个股资金流中收集的候选上限"},
		{Group: "candidate", GroupLabel: "候选生成", Key: "candidate.max_stocks_per_hotspot_soft", Label: "单热点软上限", Unit: "个", Description: "优先保证单一热点最多推荐数量，不足时可递补放宽"},

		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.factor_score_cap", Label: "因子封顶", Unit: "分", Description: "情绪因子正向最高分"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.news_evidence_score", Label: "单条新闻热度分", Unit: "分", Description: "每条证据新闻贡献的热点分"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.keyword_score", Label: "关键词命中分", Unit: "分", Description: "每个热点关键词贡献的分值"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.negative_news_penalty", Label: "负面新闻扣分", Unit: "分/条", Description: "每条负面新闻扣分"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.hotspot_min_score", Label: "热点最低分", Unit: "分", Description: "热点分低于该值时按该值展示"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.hotspot_display_limit", Label: "热点展示上限", Unit: "个", Description: "页面和推荐前置热点展示数量"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.stock_evidence_score", Label: "个股证据加分", Unit: "分/条", Description: "个股有效新闻证据加分"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.stock_name_keyword_score", Label: "股票名命中加分", Unit: "分/个", Description: "股票名命中关键词加分"},
		{Group: "emotion", GroupLabel: "情绪因子", Key: "emotion.weak_evidence_penalty", Label: "弱证据扣分", Unit: "分", Description: "融资融券等弱新闻证据扣分"},

		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.factor_score_cap", Label: "因子封顶", Unit: "分", Description: "版块资金因子正向最高分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_min_days", Label: "板块趋势最少天数", Unit: "天", Description: "板块资金趋势判断所需最少数据天数"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_score_min", Label: "板块趋势最低分", Unit: "分", Description: "板块趋势分下限"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_score_max", Label: "板块趋势最高分", Unit: "分", Description: "板块趋势分上限"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_turn_threshold", Label: "板块近2日拐点阈值", Unit: "元", Step: "1000000", Description: "板块近2日连续流入加分阈值"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_continuous_inflow_score", Label: "板块连续流入加分", Unit: "分", Description: "近5日板块连续流入加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_partial_inflow_score", Label: "板块偏流入加分", Unit: "分", Description: "近5日板块偏流入加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_weak_inflow_score", Label: "板块弱流入加分", Unit: "分", Description: "板块弱流入加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_continuous_outflow_penalty", Label: "板块连续流出扣分", Unit: "分", Description: "近5日板块连续流出扣分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_partial_outflow_penalty", Label: "板块偏流出扣分", Unit: "分", Description: "近5日板块偏流出扣分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_weak_outflow_penalty", Label: "板块弱流出扣分", Unit: "分", Description: "板块弱流出扣分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_ten_day_acceleration_score", Label: "板块10日加速加分", Unit: "分", Description: "板块资金10日加速流入加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_ten_day_outflow_penalty", Label: "板块10日流出扣分", Unit: "分", Description: "板块10日和5日均流出扣分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_recent_2d_inflow_score", Label: "板块近2日流入加分", Unit: "分", Description: "板块近2日连续流入加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_recent_2d_outflow_penalty", Label: "板块近2日流出扣分", Unit: "分", Description: "板块近2日连续流出扣分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_rank_top10_score", Label: "板块排名前10加分", Unit: "分", Description: "板块资金排名前10加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_rank_top30_score", Label: "板块排名前30加分", Unit: "分", Description: "板块资金排名前30加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_latest_outflow_down_penalty", Label: "板块当日流出下跌扣分", Unit: "分", Description: "板块当日净流出且下跌扣分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.trend_choppy_penalty", Label: "板块资金震荡扣分", Unit: "分", Description: "板块资金方向震荡扣分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.top_stock_one_sector_score", Label: "共振1板块加分", Unit: "分", Description: "股票为1个正向板块代表股时加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.top_stock_two_sector_score", Label: "共振2板块加分", Unit: "分", Description: "股票为2个正向板块代表股时加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.top_stock_three_sector_score", Label: "共振3板块加分", Unit: "分", Description: "股票为3个及以上正向板块代表股时加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.top_stock_large_inflow_threshold", Label: "共振大额流入阈值", Unit: "元", Step: "100000000", Description: "多个板块合计流入超过该值时额外加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.top_stock_large_inflow_score", Label: "共振大额流入加分", Unit: "分", Description: "共振板块合计大额流入额外加分"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.top_stock_score_cap", Label: "共振总分封顶", Unit: "分", Description: "板块资金共振加分上限"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.top_stock_overheat_30_cap", Label: "30日过热共振封顶", Unit: "分", Description: "30日涨幅过热后共振加分封顶"},
		{Group: "sector", GroupLabel: "版块资金因子", Key: "sector.drawdown_penalty", Label: "板块回撤扣分", Unit: "分", Description: "同热点股票回撤过滤后对板块扣分"},

		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.factor_score_cap", Label: "因子封顶", Unit: "分", Description: "个股资金因子正向最高分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.bonus_threshold", Label: "5日流入一档阈值", Unit: "元", Step: "1000000", Description: "达到后给一档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.bonus_score", Label: "5日流入一档加分", Unit: "分", Description: "5日流入一档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.strong_bonus_threshold", Label: "5日流入二档阈值", Unit: "元", Step: "1000000", Description: "达到后给二档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.strong_bonus_score", Label: "5日流入二档加分", Unit: "分", Description: "5日流入二档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.very_strong_threshold", Label: "5日流入三档阈值", Unit: "元", Step: "1000000", Description: "达到后给三档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.very_strong_score", Label: "5日流入三档加分", Unit: "分", Description: "5日流入三档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.extreme_threshold", Label: "5日流入四档阈值", Unit: "元", Step: "1000000", Description: "达到后给四档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.extreme_score", Label: "5日流入四档加分", Unit: "分", Description: "5日流入四档资金加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.negative_days_2_penalty", Label: "近5日流出2天扣分", Unit: "分", Description: "近5日净流出2天扣分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.negative_days_3_penalty", Label: "近5日流出3天扣分", Unit: "分", Description: "近5日净流出3天扣分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.negative_days_4_penalty", Label: "近5日流出4天扣分", Unit: "分", Description: "近5日净流出4天及以上扣分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.ten_day_acceleration_ratio", Label: "10日加速比例", Unit: "", Step: "0.01", Description: "5日流入占10日流入比例达到后加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.ten_day_acceleration_score", Label: "10日加速加分", Unit: "分", Description: "资金10日加速流入加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.ten_day_retreat_penalty", Label: "10日转退潮扣分", Unit: "分", Description: "10日净流入但5日转流出扣分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.ten_day_repair_cap", Label: "10日流出修复封顶", Unit: "分", Description: "10日流出但5日转流入时加分上限"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.recent_turn_threshold", Label: "近2日拐点阈值", Unit: "元", Step: "1000000", Description: "近2日连续流入加分阈值"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.recent_2d_inflow_score", Label: "近2日流入加分", Unit: "分", Description: "近2日连续流入加分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.recent_2d_outflow_penalty", Label: "近2日流出扣分", Unit: "分", Description: "近2日连续流出扣分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.latest_outflow_down_penalty", Label: "当日流出下跌扣分", Unit: "分", Description: "当日净流出且股价下跌扣分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.score_min", Label: "资金最低分", Unit: "分", Description: "最终资金分下限"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.score_max", Label: "资金最高分", Unit: "分", Description: "最终资金分上限"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.sector_weak_cap", Label: "板块走弱资金封顶", Unit: "分", Description: "板块当天走弱时个股资金加分封顶"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.sector_net_outflow_cap", Label: "板块流出资金封顶", Unit: "分", Description: "板块当日净流出时个股资金加分封顶"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.median_penalty", Label: "资金低于中位数扣分", Unit: "分", Description: "同热点中资金弱于中位数时扣分"},
		{Group: "fund", GroupLabel: "个股资金因子", Key: "fund.overheat_30_cap", Label: "30日过热资金封顶", Unit: "分", Description: "30日涨幅过热后资金加分封顶"},

		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.factor_score_cap", Label: "因子封顶", Unit: "分", Description: "波动因子正向最高分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.drawdown_filter_threshold", Label: "回撤过滤阈值", Unit: "%", Step: "0.01", Description: "30/60日涨幅低于该值时过滤"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.overheat_30_threshold_pct", Label: "30日过热阈值", Unit: "%", Step: "0.01", Description: "超过后限制资金和共振加分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.overheat_60_threshold_pct", Label: "60日剔除阈值", Unit: "%", Step: "0.01", Description: "超过后直接剔除"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.previous_limit_up_penalty", Label: "昨日涨停扣分", Unit: "分", Description: "昨日涨停风险扣分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.previous_high_pct_threshold", Label: "昨日高涨幅阈值", Unit: "%", Step: "0.01", Description: "昨日涨幅超过该值时扣分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.previous_high_pct_penalty", Label: "昨日高涨幅扣分", Unit: "分", Description: "昨日涨幅过高风险扣分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.today_high_pct_filter_threshold", Label: "今日涨幅过滤阈值", Unit: "%", Step: "0.01", Description: "今日涨幅超过该值时过滤"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.momentum_min_bars", Label: "动量最少K线", Unit: "根", Description: "动量评分所需最少K线数量"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.momentum_max_score", Label: "动量最高分", Unit: "分", Description: "动量趋势总分上限"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.momentum_adx_score", Label: "ADX转强加分", Unit: "分", Description: "ADX转强信号加分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.momentum_bollinger_score", Label: "布林突破加分", Unit: "分", Description: "布林突破信号加分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.momentum_macd_score", Label: "MACD转强加分", Unit: "分", Description: "MACD转强信号加分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.momentum_ma_score", Label: "均线趋势加分", Unit: "分", Description: "均线趋势确认加分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.momentum_volume_score", Label: "放量确认加分", Unit: "分", Description: "放量确认信号加分"},
		{Group: "volatility", GroupLabel: "波动因子", Key: "volatility.ex_dividend_window_days", Label: "除权除息窗口", Unit: "天", Description: "当日及未来N天除权除息过滤窗口"},
	}
}

func aStockAlgorithmFieldString(settings model.AStockRecommendationAlgorithmSettings, key string) string {
	field, ok := aStockAlgorithmField(reflect.ValueOf(settings), key)
	if !ok {
		return ""
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(field.Int(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(field.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(field.Bool())
	case reflect.String:
		return field.String()
	default:
		return ""
	}
}

func setAStockAlgorithmFieldString(settings *model.AStockRecommendationAlgorithmSettings, key string, raw string) error {
	field, ok := aStockAlgorithmField(reflect.ValueOf(settings).Elem(), key)
	if !ok || !field.CanSet() {
		return fmt.Errorf("unknown field %s", key)
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		field.SetFloat(value)
	case reflect.Bool:
		field.SetBool(normalizeAStockBool(raw))
	case reflect.String:
		field.SetString(raw)
	default:
		return fmt.Errorf("unsupported field type %s", field.Kind())
	}
	return nil
}

func aStockAlgorithmField(root reflect.Value, key string) (reflect.Value, bool) {
	current := root
	if current.Kind() == reflect.Pointer {
		current = current.Elem()
	}
	for _, part := range strings.Split(key, ".") {
		if current.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		next, ok := aStockAlgorithmStructFieldByJSONTag(current, part)
		if !ok {
			return reflect.Value{}, false
		}
		current = next
		if current.Kind() == reflect.Pointer {
			current = current.Elem()
		}
	}
	return current, true
}

func aStockAlgorithmStructFieldByJSONTag(value reflect.Value, tagName string) (reflect.Value, bool) {
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldInfo := valueType.Field(i)
		jsonName := strings.Split(fieldInfo.Tag.Get("json"), ",")[0]
		if jsonName == tagName {
			return value.Field(i), true
		}
	}
	return reflect.Value{}, false
}
