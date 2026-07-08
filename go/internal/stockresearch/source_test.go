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
