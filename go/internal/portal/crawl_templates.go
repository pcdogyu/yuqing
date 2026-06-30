package portal

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

func (s *Server) handleCrawlTemplatesPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		message := s.handleCrawlTemplateAction(r)
		target := url.Values{}
		target.Set("msg", message)
		http.Redirect(w, r, "/crawl-templates/manage?"+target.Encode(), http.StatusSeeOther)
		return
	}

	templates := s.loadAllCrawlTemplates()
	enabledCount := 0
	selectedTemplateID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("template_id")), 10, 64)
	var selectedTemplate model.CrawlTemplate
	selectedTemplateFound := false
	for _, tpl := range templates {
		if tpl.Enabled {
			enabledCount++
		}
		if selectedTemplateID > 0 && tpl.ID == selectedTemplateID {
			selectedTemplate = tpl
			selectedTemplateFound = true
		}
	}

	var body strings.Builder
	body.WriteString(`<style>
		input,select,textarea,button{width:100%;padding:12px;margin:8px 0;border-radius:10px;border:1px solid #d0c8b8;box-sizing:border-box}
		button{background:#214e34;color:#fff;border:none;cursor:pointer}
		.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}
		.subtle{color:#6a6257}
		.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}
		.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}
		.summary-card strong{display:block;font-size:24px;margin-top:6px}
		.template-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(360px,1fr));gap:16px}
		.template-card{border:1px solid #ece7dc;border-radius:14px;padding:16px;background:#faf8f2}
		.template-card form{margin:0}
		.template-meta{color:#6a6257;font-size:13px}
		.param-panel{margin:12px 0;padding:12px;border:1px solid #e5dece;border-radius:10px;background:#fff}
		.param-panel h3{font-size:16px;margin:0 0 10px}
		.param-list{display:grid;grid-template-columns:minmax(86px,120px) minmax(0,1fr);gap:8px 12px;margin:0}
		.param-list dt{color:#6a6257;font-size:13px}
		.param-list dd{margin:0;word-break:break-all}
		.template-actions{display:flex;gap:10px;flex-wrap:wrap;margin-top:8px}
		.template-actions button{width:auto;min-width:120px}
		.template-actions a{display:inline-flex;align-items:center;justify-content:center;padding:10px 14px;border-radius:10px;background:#214e34;color:#fff;text-decoration:none;font-weight:700}
		.template-actions a.secondary{background:#efe9dc;color:#214e34}
		.template-actions .danger{background:#8f2d2d}
	</style>`)
	body.WriteString(`<section><p class="subtle">欢迎，用户 `)
	body.WriteString(strconv.FormatInt(userIDFromMap(user), 10))
	body.WriteString(`。这里可以集中维护 X / Telegram / 资讯抓取模板。</p></section>`)

	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		body.WriteString(`<div class="msg">`)
		body.WriteString(html.EscapeString(msg))
		body.WriteString(`</div>`)
	}

	body.WriteString(`<section><h2>概览</h2><div class="summary-grid">`)
	body.WriteString(templateSummaryCard("模板总数", len(templates)))
	body.WriteString(templateSummaryCard("已启用", enabledCount))
	body.WriteString(templateSummaryCard("已停用", len(templates)-enabledCount))
	body.WriteString(`</div></section>`)

	if selectedTemplateID > 0 {
		body.WriteString(`<section><h2>模板详情</h2>`)
		if selectedTemplateFound {
			body.WriteString(crawlTemplateDetailPanel(selectedTemplate))
		} else {
			body.WriteString(`<p class="subtle">未找到模板 #`)
			body.WriteString(strconv.FormatInt(selectedTemplateID, 10))
			body.WriteString(`，请从下方模板列表重新选择。</p>`)
		}
		body.WriteString(`</section>`)
	}

	body.WriteString(`<section><h2>新建模板</h2><form method="post"><input type="hidden" name="action" value="create"><div class="template-grid">`)
	body.WriteString(`<div class="template-card"><label>模板名称</label><input name="name" placeholder="X BTC 热门账号模板"><label>网站参数</label><input class="js-website-input" name="website" list="crawl-template-website-options" placeholder="如 x.com / t.me / example.com"><label>来源类型</label><select class="js-source-type-input" name="source_type">`)
	body.WriteString(crawlTemplateSourceOptions("custom"))
	body.WriteString(`</select><label>请求方式</label><select class="js-method-input" name="config_method"><option value="GET">GET</option><option value="POST">POST</option></select><label>页面地址</label><input class="js-base-url-input" name="config_base_url" placeholder="https://example.com"><label>列表选择器</label><input class="js-list-selector-input" name="config_list_selector" placeholder=".list-item"><label>详情链接字段</label><input class="js-detail-url-field-input" name="config_detail_url_field" placeholder="href"><label>配置 JSON</label><textarea class="js-config-json-input" name="config_json" rows="12" placeholder='{"source_type":"crypto_x","base_url":"https://example.com"}'>{}</textarea><p class="template-meta">常用字段可直接填写，上面的值会自动同步回配置 JSON；如果需要停用，可在下方列表保存后切换。</p><div class="template-actions"><button type="submit">创建模板</button></div></div></div></form></section>`)

	body.WriteString(`<section><h2>模板列表（共 `)
	body.WriteString(strconv.Itoa(len(templates)))
	body.WriteString(` 个）</h2>`)
	if len(templates) == 0 {
		body.WriteString(`<p class="subtle">暂无模板，先创建一个预置模板吧。</p>`)
	} else {
		body.WriteString(`<div class="template-grid">`)
		for _, tpl := range templates {
			params := crawlTemplateDisplayParams(tpl)
			detailURL := "/crawl-templates/manage?template_id=" + strconv.FormatInt(tpl.ID, 10)
			body.WriteString(`<div class="template-card"><form method="post"><input type="hidden" name="template_id" value="`)
			body.WriteString(strconv.FormatInt(tpl.ID, 10))
			body.WriteString(`"><input type="hidden" name="action" value="update"><label>模板名称</label><input name="name" value="`)
			body.WriteString(html.EscapeString(tpl.Name))
			body.WriteString(`"><label>网站参数</label><input class="js-website-input" name="website" list="crawl-template-website-options" value="`)
			body.WriteString(html.EscapeString(tpl.Website))
			body.WriteString(`" placeholder="如 x.com / t.me / example.com"><label>来源类型</label><select class="js-source-type-input" name="source_type">`)
			body.WriteString(crawlTemplateSourceOptions(tpl.SourceType))
			body.WriteString(`</select>`)
			body.WriteString(crawlTemplateParamsPanel(params))
			body.WriteString(`<label>请求方式</label><select class="js-method-input" name="config_method"><option value="GET">GET</option><option value="POST">POST</option></select><label>页面地址</label><input class="js-base-url-input" name="config_base_url" placeholder="https://example.com"><label>列表选择器</label><input class="js-list-selector-input" name="config_list_selector" placeholder=".list-item"><label>详情链接字段</label><input class="js-detail-url-field-input" name="config_detail_url_field" placeholder="href"><label>配置 JSON</label><textarea class="js-config-json-input" name="config_json" rows="12">`)
			body.WriteString(html.EscapeString(tpl.ConfigJSON))
			body.WriteString(`</textarea><label style="display:flex;align-items:center;gap:8px;margin-top:6px"><input type="checkbox" name="enabled"`)
			if tpl.Enabled {
				body.WriteString(` checked`)
			}
			body.WriteString(`>启用</label><div class="template-meta">ID `)
			body.WriteString(strconv.FormatInt(tpl.ID, 10))
			body.WriteString(` | 创建于 `)
			body.WriteString(html.EscapeString(tpl.CreatedAt.Format("2006-01-02 15:04")))
			body.WriteString(` | 更新于 `)
			body.WriteString(html.EscapeString(tpl.UpdatedAt.Format("2006-01-02 15:04")))
			body.WriteString(`</div><div class="template-actions"><a class="secondary" href="`)
			body.WriteString(html.EscapeString(detailURL))
			body.WriteString(`">查看抓取页面</a><button type="submit">保存模板</button></div></form><form method="post"><input type="hidden" name="action" value="delete"><input type="hidden" name="template_id" value="`)
			body.WriteString(strconv.FormatInt(tpl.ID, 10))
			body.WriteString(`"><div class="template-actions"><button class="danger" type="submit" onclick="return confirm('确认删除模板：`)
			body.WriteString(html.EscapeString(tpl.Name))
			body.WriteString(`？')">删除模板</button></div></form></div>`)
		}
		body.WriteString(`</div>`)
	}
	body.WriteString(`</section>`)
	body.WriteString(crawlTemplateWebsiteDatalist())
	body.WriteString(crawlTemplateWebsiteScript())

	_ = s.writeSimplePage(w, "crawl-templates", "抓取模板管理", body.String())
}

func (s *Server) handleCrawlTemplateAction(r *http.Request) string {
	if err := r.ParseForm(); err != nil {
		return "模板操作失败"
	}
	action := strings.TrimSpace(r.FormValue("action"))
	switch action {
	case "create":
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			return "模板名称不能为空"
		}
		payload := model.CrawlTemplate{
			Name:       name,
			Website:    strings.TrimSpace(r.FormValue("website")),
			SourceType: strings.TrimSpace(r.FormValue("source_type")),
			ConfigJSON: normalizeTemplateConfigJSON(r.FormValue("config_json"), r.FormValue("website"), r.FormValue("config_method"), r.FormValue("config_base_url"), r.FormValue("config_list_selector"), r.FormValue("config_detail_url_field")),
		}
		resp, err := s.client.R().SetBody(payload).Post(s.cfg.ContentURL + "/api/v1/crawl-templates")
		if err != nil || !resp.IsSuccess() {
			return "模板创建失败"
		}
		return "模板已创建"
	case "update":
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("template_id")), 10, 64)
		if err != nil || id <= 0 {
			return "无效模板ID"
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			return "模板名称不能为空"
		}
		payload := model.CrawlTemplate{
			ID:         id,
			Name:       name,
			Website:    strings.TrimSpace(r.FormValue("website")),
			SourceType: strings.TrimSpace(r.FormValue("source_type")),
			Enabled:    r.FormValue("enabled") == "on",
			ConfigJSON: normalizeTemplateConfigJSON(r.FormValue("config_json"), r.FormValue("website"), r.FormValue("config_method"), r.FormValue("config_base_url"), r.FormValue("config_list_selector"), r.FormValue("config_detail_url_field")),
		}
		resp, err := s.client.R().SetBody(payload).Put(s.cfg.ContentURL + "/api/v1/crawl-templates/" + strconv.FormatInt(id, 10))
		if err != nil || !resp.IsSuccess() {
			return "模板保存失败"
		}
		return "模板已保存"
	case "delete":
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("template_id")), 10, 64)
		if err != nil || id <= 0 {
			return "无效模板ID"
		}
		resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/crawl-templates/" + strconv.FormatInt(id, 10))
		if err != nil || !resp.IsSuccess() {
			return "模板删除失败"
		}
		return "模板已删除"
	default:
		return "未知操作"
	}
}

func (s *Server) loadAllCrawlTemplates() []model.CrawlTemplate {
	var templates []model.CrawlTemplate
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/crawl-templates", &templates); err != nil {
		return nil
	}
	return templates
}

func crawlTemplateSourceOptions(selected string) string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		selected = "custom"
	} else if canonical := provider.ValidSourceType(selected); canonical != "" {
		selected = canonical
	}
	options := []struct {
		value string
		label string
	}{
		{provider.SourceTypeFlash, "jin10_kuaixun"},
		{provider.SourceTypeHeadline, "jin10_资讯"},
		{provider.SourceTypeJin10Full, "金十公开资讯全量"},
		{provider.SourceTypeEastMoneyKuaixun, "东方财富网"},
		{provider.SourceTypeCryptoX, provider.SourceTypeCryptoX},
		{provider.SourceTypeCryptoTelegram, provider.SourceTypeCryptoTelegram},
		{provider.SourceTypeForesightNewsflash, provider.SourceTypeForesightNewsflash},
		{provider.SourceTypeCoinDeskZHLatest, provider.SourceTypeCoinDeskZHLatest},
		{provider.SourceTypePANewsNewsflash, provider.SourceTypePANewsNewsflash},
		{provider.SourceTypeTheBlockLatest, provider.SourceTypeTheBlockLatest},
		{"custom", "custom"},
	}
	var b strings.Builder
	for _, option := range options {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(option.value))
		b.WriteString(`"`)
		if option.value == selected {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(option.label))
		b.WriteString(`</option>`)
	}
	return b.String()
}

func normalizeTemplateConfigJSON(raw string, website string, method string, baseURL string, listSelector string, detailURLField string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	website = strings.TrimSpace(website)
	method = strings.ToUpper(strings.TrimSpace(method))
	baseURL = strings.TrimSpace(baseURL)
	listSelector = strings.TrimSpace(listSelector)
	detailURLField = strings.TrimSpace(detailURLField)
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}
	if website == "" {
		delete(payload, "website")
	} else {
		payload["website"] = website
	}
	if method == "" || method == "GET" {
		delete(payload, "method")
	} else {
		payload["method"] = method
	}
	if baseURL == "" {
		delete(payload, "base_url")
	} else {
		payload["base_url"] = baseURL
	}
	if listSelector == "" {
		delete(payload, "list_selector")
	} else {
		payload["list_selector"] = listSelector
	}
	if detailURLField == "" {
		delete(payload, "detail_url_field")
	} else {
		payload["detail_url_field"] = detailURLField
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return string(normalized)
}

type crawlTemplateParam struct {
	Label string
	Value string
}

func crawlTemplateDisplayParams(tpl model.CrawlTemplate) []crawlTemplateParam {
	cfg, raw := parseCrawlTemplateConfig(tpl.ConfigJSON)
	website := strings.TrimSpace(tpl.Website)
	if website == "" {
		website = stringMapValue(raw, "website")
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = "GET"
	}
	sourceType := strings.TrimSpace(tpl.SourceType)
	if sourceType == "" {
		sourceType = strings.TrimSpace(cfg.SourceType)
	}
	return []crawlTemplateParam{
		{Label: "网站", Value: website},
		{Label: "来源类型", Value: sourceType},
		{Label: "请求方式", Value: method},
		{Label: "URL", Value: cfg.BaseURL},
		{Label: "抓取区域", Value: cfg.ListSelector},
		{Label: "详情区域", Value: cfg.DetailSelector},
		{Label: "href 字段", Value: cfg.DetailURLField},
		{Label: "分页", Value: crawlTemplatePaginationText(cfg.Pagination)},
		{Label: "字段映射", Value: crawlTemplateFieldsText(cfg.Fields)},
	}
}

func crawlTemplateParamsPanel(params []crawlTemplateParam) string {
	var b strings.Builder
	b.WriteString(`<div class="param-panel"><h3>抓取参数</h3><dl class="param-list">`)
	for _, param := range params {
		b.WriteString(`<dt>`)
		b.WriteString(html.EscapeString(param.Label))
		b.WriteString(`</dt><dd>`)
		b.WriteString(html.EscapeString(displayValue(param.Value)))
		b.WriteString(`</dd>`)
	}
	b.WriteString(`</dl></div>`)
	return b.String()
}

func crawlTemplateDetailPanel(tpl model.CrawlTemplate) string {
	cfg, raw := parseCrawlTemplateConfig(tpl.ConfigJSON)
	pageURL := strings.TrimSpace(cfg.BaseURL)
	if pageURL == "" {
		pageURL = stringMapValue(raw, "url")
	}
	if pageURL == "" {
		pageURL = stringMapValue(raw, "page_url")
	}
	var b strings.Builder
	b.WriteString(`<div class="template-card"><h3>`)
	b.WriteString(html.EscapeString(tpl.Name))
	b.WriteString(`</h3><div class="param-panel"><h3>抓取页面</h3><dl class="param-list"><dt>页面地址</dt><dd>`)
	if pageURL == "" {
		b.WriteString(`--`)
	} else {
		b.WriteString(`<a class="inline" href="`)
		b.WriteString(html.EscapeString(pageURL))
		b.WriteString(`" target="_blank" rel="noreferrer">`)
		b.WriteString(html.EscapeString(pageURL))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</dd><dt>模板 ID</dt><dd>`)
	b.WriteString(strconv.FormatInt(tpl.ID, 10))
	b.WriteString(`</dd><dt>网站</dt><dd>`)
	b.WriteString(html.EscapeString(displayValue(firstNonEmpty(tpl.Website, stringMapValue(raw, "website")))))
	b.WriteString(`</dd><dt>来源类型</dt><dd>`)
	b.WriteString(html.EscapeString(displayValue(firstNonEmpty(tpl.SourceType, cfg.SourceType))))
	b.WriteString(`</dd><dt>请求方式</dt><dd>`)
	b.WriteString(html.EscapeString(displayValue(firstNonEmpty(strings.ToUpper(strings.TrimSpace(cfg.Method)), "GET"))))
	b.WriteString(`</dd><dt>列表选择器</dt><dd>`)
	b.WriteString(html.EscapeString(displayValue(cfg.ListSelector)))
	b.WriteString(`</dd><dt>详情链接字段</dt><dd>`)
	b.WriteString(html.EscapeString(displayValue(cfg.DetailURLField)))
	b.WriteString(`</dd></dl></div><div class="template-actions"><a class="secondary" href="/crawl-templates/manage">返回模板列表</a></div></div>`)
	return b.String()
}

func parseCrawlTemplateConfig(raw string) (model.CrawlTemplateConfig, map[string]any) {
	var cfg model.CrawlTemplateConfig
	payload := map[string]any{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg, payload
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, payload
	}
	_ = json.Unmarshal([]byte(raw), &payload)
	return cfg, payload
}

func stringMapValue(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func displayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "--"
	}
	return value
}

func crawlTemplatePaginationText(p model.CrawlTemplatePagination) string {
	if !p.Enabled {
		return ""
	}
	parts := []string{"启用"}
	if strings.TrimSpace(p.Param) != "" {
		parts = append(parts, "参数 "+p.Param)
	}
	if p.End > 0 {
		parts = append(parts, "范围 "+strconv.Itoa(p.Start)+"-"+strconv.Itoa(p.End))
	}
	if p.Step > 0 {
		parts = append(parts, "步长 "+strconv.Itoa(p.Step))
	}
	return strings.Join(parts, "，")
}

func crawlTemplateFieldsText(fields []model.CrawlTemplateField) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
		target := strings.TrimSpace(field.Selector)
		if strings.TrimSpace(field.Attr) != "" {
			target += "@" + strings.TrimSpace(field.Attr)
		}
		if target == "" {
			target = strings.TrimSpace(field.Value)
		}
		if target == "" {
			target = strings.TrimSpace(field.From)
		}
		if target == "" {
			parts = append(parts, name)
			continue
		}
		parts = append(parts, name+": "+target)
	}
	return strings.Join(parts, "；")
}

func templateSummaryCard(label string, value int) string {
	return `<div class="summary-card"><div class="template-meta">` + html.EscapeString(label) + `</div><strong>` + strconv.Itoa(value) + `</strong></div>`
}

func crawlTemplateWebsiteDatalist() string {
	return `<datalist id="crawl-template-website-options"><option value="x.com"></option><option value="twitter.com"></option><option value="t.me"></option><option value="telegram.org"></option><option value="jin10.com"></option><option value="xnews.jin10.com"></option><option value="flash.jin10.com"></option><option value="kuaixun.eastmoney.com"></option><option value="foresightnews.pro"></option><option value="coindesk.com"></option><option value="panewslab.com"></option><option value="theblock.co"></option><option value="example.com"></option></datalist>`
}

func crawlTemplateWebsiteScript() string {
	return `<script>(function(){var defaults={jin10_kuaixun:"flash.jin10.com","jin10_资讯":"xnews.jin10.com",flash:"flash.jin10.com",headline:"xnews.jin10.com",jin10_full:"jin10.com",eastmoney_kuaixun:"kuaixun.eastmoney.com",wallstreetcn_a_stock:"wallstreetcn.com",cls_telegraph:"cls.cn",sina_finance_7x24:"finance.sina.com.cn",crypto_x:"x.com",crypto_telegram:"t.me",foresight_newsflash:"foresightnews.pro",coindesk_zh_latest:"coindesk.com",panews_newsflash:"panewslab.com",theblock_latest:"theblock.co"};var parseConfig=function(text){try{return JSON.parse(text||"{}")}catch(e){return {}}};document.querySelectorAll("form").forEach(function(form){var source=form.querySelector(".js-source-type-input");var website=form.querySelector(".js-website-input");var method=form.querySelector(".js-method-input");var baseUrl=form.querySelector(".js-base-url-input");var listSelector=form.querySelector(".js-list-selector-input");var detailUrlField=form.querySelector(".js-detail-url-field-input");var config=form.querySelector(".js-config-json-input");if(!source||!website||!method||!baseUrl||!listSelector||!detailUrlField||!config){return}var hydrate=function(){var parsed=parseConfig(config.value);if(!method.value&&parsed.method){method.value=String(parsed.method).toUpperCase()}if(method.value===""){method.value="GET"}if(!baseUrl.value&&parsed.base_url){baseUrl.value=parsed.base_url}if(!listSelector.value&&parsed.list_selector){listSelector.value=parsed.list_selector}if(!detailUrlField.value&&parsed.detail_url_field){detailUrlField.value=parsed.detail_url_field}if(!website.value&&parsed.website){website.value=parsed.website}};var applyDefaultWebsite=function(){if(website.value.trim()!==""||!defaults[source.value]){return}website.value=defaults[source.value]};source.addEventListener("change",applyDefaultWebsite);hydrate();applyDefaultWebsite()})})();</script>`
}
