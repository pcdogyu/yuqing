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
