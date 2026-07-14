package stockresearch

import (
	"strings"
	"testing"
)

func TestExtractHTMLSourceTextPreservesReadableFormatting(t *testing.T) {
	text := ExtractHTMLSourceText(`<html><body><article>
<h1>标题</h1>
<p>第一段  保留空格</p>
<p>第二段<br>换行</p>
<ul><li>列表项 A</li><li>列表项 B</li></ul>
<table><tr><th>项目</th><td>金额  100</td></tr></table>
</article></body></html>`)

	for _, want := range []string{"标题", "第一段  保留空格", "第二段\n换行", "列表项 A", "列表项 B", "项目    金额  100"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected extracted text to contain %q, got %q", want, text)
		}
	}
	if strings.Contains(text, "\n\n\n") {
		t.Fatalf("expected blank lines to be compressed, got %q", text)
	}
}

func TestCleanSinaFinanceReportTextRemovesNavigationAndRecommendations(t *testing.T) {
	raw := strings.Join([]string{
		"研究报告",
		"最新滚动",
		"主力动向",
		"个股评级",
		"公司研究",
		"行业研究",
		"投资策略",
		"宏观研究",
		"金麒麟研报",
		"更多",
		"晨报",
		"创业板",
		"基金",
		"债券",
		"金融工程",
		"个股点评",
		"百隆东方(601339)：期待下半年价增驱动成长",
		"公司发布26 年半年度业绩预告",
		"风险提示：外贸环境变化，原材料价格波动。",
		"数据推荐",
		"最新投资评级",
		"目标涨幅排名",
		"上调投资评级",
		"下调投资评级",
		"机构关注度",
		"行业关注度",
		"股票综合评级",
		"首次评级股票",
	}, "\n\n")

	text := CleanSourceTextForURL("https://stock.finance.sina.com.cn/stock/go.php/vReport_Show/kind/company/rptid/837081332181/index.phtml", raw)

	for _, unwanted := range []string{"研究报告", "最新滚动", "主力动向", "个股评级", "数据推荐", "最新投资评级", "首次评级股票"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("expected cleaned sina text not to contain %q, got %q", unwanted, text)
		}
	}
	for _, want := range []string{"百隆东方(601339)：期待下半年价增驱动成长", "公司发布26 年半年度业绩预告", "风险提示：外贸环境变化，原材料价格波动。"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected cleaned sina text to contain %q, got %q", want, text)
		}
	}
}

func TestCleanEastMoneyReportTextRemovesNavigationAndTail(t *testing.T) {
	raw := strings.Join([]string{
		"财经 焦点 股票 新股 研报 个股研报 行业研报 盈利预测",
		"电磁线领域领先龙头，行业高景气+全球化布局驱动新成长",
		"宏远股份(920018) 投资要点",
		"深耕高性能电磁线，全球化布局打开成长空间。",
		"盈利预测与投资评级：预计公司业绩稳步增长。",
		"风险提示：下游行业需求波动，原材料价格波动。",
		"调高投资评级",
		"调低投资评级",
		"首次评级股票",
		"盈利预测排行",
		"最新研究报告",
		"买入评级个股",
		"东吴证券",
		"国金证券",
		"数据来源：东方财富Choice数据",
		"郑重声明：东方财富网发布此信息",
		"东方财富免费版",
	}, "\n\n")

	text := CleanEastMoneyReportText(raw)

	if !strings.HasPrefix(text, "宏远股份(920018) 投资要点") {
		t.Fatalf("expected eastmoney text to start at investment points anchor, got %q", text)
	}
	for _, unwanted := range []string{"财经 焦点 股票", "电磁线领域领先龙头", "调高投资评级", "调低投资评级", "首次评级股票", "数据来源：东方财富Choice数据", "郑重声明", "东方财富免费版"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("expected cleaned eastmoney text not to contain %q, got %q", unwanted, text)
		}
	}
	for _, want := range []string{"深耕高性能电磁线", "盈利预测与投资评级", "风险提示：下游行业需求波动"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected cleaned eastmoney text to contain %q, got %q", want, text)
		}
	}
}

func TestCleanSourceTextForURLAppliesEastMoneyRules(t *testing.T) {
	raw := "财经 焦点 股票\n\n宏远股份(920018) 投资要点\n\n正文内容\n\n数据来源：东方财富Choice数据\n\n尾部"

	text := CleanSourceTextForURL("https://data.eastmoney.com/report/info/AP202607121826913867.html", raw)

	if !strings.HasPrefix(text, "宏远股份(920018) 投资要点") {
		t.Fatalf("expected eastmoney URL cleaner to trim leading navigation, got %q", text)
	}
	if strings.Contains(text, "财经 焦点 股票") || strings.Contains(text, "数据来源：东方财富Choice数据") || strings.Contains(text, "尾部") {
		t.Fatalf("expected eastmoney URL cleaner to remove noise, got %q", text)
	}
	if !strings.Contains(text, "正文内容") {
		t.Fatalf("expected eastmoney URL cleaner to keep body, got %q", text)
	}
}

func TestCleanSourceTextForURLDoesNotApplySinaRulesToOtherSources(t *testing.T) {
	raw := "研究报告\n\n正文\n\n数据推荐\n\n尾部"

	text := CleanSourceTextForURL("https://example.com/report.html", raw)

	for _, want := range []string{"研究报告", "正文", "数据推荐", "尾部"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected non-sina text to keep %q, got %q", want, text)
		}
	}
}

func TestFormatPDFTextPreservesLineBreaks(t *testing.T) {
	text := FormatPDFText("  # PDF 原文  \n\n第一段\n第二段\n\n\n第三段  ")

	for _, want := range []string{"# PDF 原文", "第一段\n第二段", "第二段\n\n第三段"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected formatted PDF text to contain %q, got %q", want, text)
		}
	}
	if strings.Contains(text, "\n\n\n") {
		t.Fatalf("expected repeated blank lines to be compressed, got %q", text)
	}
	if strings.Contains(text, "第一段 第二段") {
		t.Fatalf("expected PDF text not to be collapsed into one line, got %q", text)
	}
}
