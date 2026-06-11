package portal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

const xieReportPromptID = 82
const xieMaxInputLength = 1000

type legacyResultUtil struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   any    `json:"data"`
}

type legacyCopyWriting struct {
	PromptID        int               `json:"promptId"`
	Temperature     float64           `json:"temperature"`
	KnowledgeBaseID string            `json:"knowledgeBaseId"`
	Params          map[string]string `json:"params"`
}

type publicOptionPageData struct {
	UserID    int64
	Options   []model.PublicOption
	Option    model.PublicOption
	Title     string
	BodyTitle string
	Message   string
}

func (s *Server) handlePlatformCompat(w http.ResponseWriter, r *http.Request, user any) {
	path := strings.TrimPrefix(r.URL.Path, "/platform/")
	switch {
	case path == "", path == "/":
		s.handlePlatformBindingsPage(w, r, user)
	case path == "bindings":
		s.handlePlatformBindingsPage(w, r, user)
	case path == "notice":
		s.handlePlatformNotice(w, r)
	case path == "nlp/bind":
		s.handlePlatformNLPBind(w, r, user)
	case path == "nlp/ocr":
		s.handlePlatformNLPImageCompat(w, r, user, "ocr")
	case path == "nlp/image":
		s.handlePlatformNLPImageCompat(w, r, user, "image")
	case path == "xie/bind":
		s.handlePlatformXieBind(w, r, user)
	case path == "xie/checkBind":
		s.handlePlatformXieCheckBind(w, r, user)
	case strings.HasPrefix(path, "xie/title/"):
		s.handlePlatformXieTitle(w, r, user, strings.TrimPrefix(path, "xie/title/"))
	case strings.HasPrefix(path, "xie/report/"):
		s.handlePlatformXieReportArticle(w, r, user, strings.TrimPrefix(path, "xie/report/"))
	case path == "xie/report":
		s.handlePlatformXieReportQuery(w, r, user)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handlePlatformBindingsPage(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		formType := strings.TrimSpace(r.FormValue("form_type"))
		if formType == "nlp_ocr" || formType == "nlp_image" || formType == "xie_title" || formType == "xie_report" {
			s.handlePlatformWorkbenchAction(w, r, user, formType)
			return
		}
		kind := strings.TrimSpace(r.FormValue("kind"))
		if kind != "nlp" && kind != "xie" {
			http.Redirect(w, r, "/platform/bindings?msg="+url.QueryEscape("未知绑定类型"), http.StatusSeeOther)
			return
		}
		binding := model.PlatformBinding{
			UserID:    userID,
			Kind:      kind,
			SecretID:  strings.TrimSpace(firstNonEmpty(r.FormValue("secret_id"), r.FormValue("secretId"))),
			SecretKey: strings.TrimSpace(firstNonEmpty(r.FormValue("secret_key"), r.FormValue("secretKey"))),
			Bound:     r.FormValue("bound") == "on" || strings.EqualFold(r.FormValue("bound"), "true"),
		}
		s.writePlatformAuditLog(user, "platform.binding.save", kind, map[string]any{
			"bound":     binding.Bound,
			"secret_id": binding.SecretID,
		})
		if _, err := s.putPlatformBinding(binding); err != nil {
			http.Redirect(w, r, "/platform/bindings?msg="+url.QueryEscape("绑定保存失败"), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/platform/bindings?msg="+url.QueryEscape("绑定已保存"), http.StatusSeeOther)
		return
	}
	s.renderPlatformWorkbench(w, r, user, pageData{
		Title:                "平台工作台",
		Message:              r.URL.Query().Get("msg"),
		PlatformImageURL:     strings.TrimSpace(r.URL.Query().Get("imageUrl")),
		PlatformXieDraftText: strings.TrimSpace(r.URL.Query().Get("text")),
		PlatformXieArticleID: strings.TrimSpace(r.URL.Query().Get("articleId")),
	})
	return
	nlpBinding, _ := s.getPlatformBinding(userID, "nlp")
	xieBinding, _ := s.getPlatformBinding(userID, "xie")
	_ = s.render(w, "platform_bindings_v2", pageData{
		Title:              "平台绑定",
		User:               user,
		Message:            r.URL.Query().Get("msg"),
		Notices:            s.loadPlatformNotices(),
		AuditLogs:          s.loadPlatformAuditLogs(userID),
		PlatformNLPBinding: nlpBinding,
		PlatformXieBinding: xieBinding,
	})
}

func (s *Server) renderPlatformWorkbench(w http.ResponseWriter, r *http.Request, user any, data pageData) {
	userID := userIDFromMap(user)
	nlpBinding, _ := s.getPlatformBinding(userID, "nlp")
	xieBinding, _ := s.getPlatformBinding(userID, "xie")
	data.Title = nonEmpty(data.Title, "平台工作台")
	data.User = user
	data.Notices = s.loadPlatformNotices()
	data.AuditLogs = s.loadPlatformAuditLogs(userID)
	data.PlatformNLPBinding = nlpBinding
	data.PlatformXieBinding = xieBinding
	_ = s.render(w, "platform_bindings_v2", data)
}

func (s *Server) handlePlatformWorkbenchAction(w http.ResponseWriter, r *http.Request, user any, formType string) {
	userID := userIDFromMap(user)
	state := pageData{
		Title:                "平台工作台",
		PlatformImageURL:     strings.TrimSpace(firstNonEmpty(r.FormValue("imageUrl"), r.FormValue("image_url"))),
		PlatformXieArticleID: strings.TrimSpace(firstNonEmpty(r.FormValue("article_id"), r.FormValue("articleId"))),
		PlatformXieDraftText: strings.TrimSpace(firstNonEmpty(r.FormValue("text"), r.FormValue("content"), r.FormValue("summary"))),
	}
	switch formType {
	case "nlp_ocr":
		text, err := s.runPlatformOCRWorkbench(r, userID)
		if err != nil {
			state.Message = err.Error()
		} else {
			state.Message = "OCR 识别已完成"
			state.PlatformOCRText = text
			s.writePlatformAuditLog(user, "platform.nlp.ocr", "ocr", map[string]any{"image_url": state.PlatformImageURL})
		}
	case "nlp_image":
		keywords, err := s.runPlatformImageWorkbench(r, userID)
		if err != nil {
			state.Message = err.Error()
		} else {
			state.Message = "图像识别已完成"
			state.PlatformImageKeywords = keywords
			s.writePlatformAuditLog(user, "platform.nlp.image", "image", map[string]any{"image_url": state.PlatformImageURL})
		}
	case "xie_title":
		title, err := s.runPlatformXieTitleWorkbench(r, userID)
		if err != nil {
			state.Message = err.Error()
		} else {
			state.Message = "标题已生成"
			state.PlatformXieGeneratedTitle = title
			s.writePlatformAuditLog(user, "platform.xie.title", nonEmpty(state.PlatformXieArticleID, "manual"), map[string]any{"article_id": state.PlatformXieArticleID})
		}
	case "xie_report":
		title, report, err := s.runPlatformXieReportWorkbench(r, userID)
		if err != nil {
			state.Message = err.Error()
		} else {
			state.Message = "报告预览已生成"
			state.PlatformXieGeneratedTitle = title
			state.PlatformXieGeneratedReport = report
			s.writePlatformAuditLog(user, "platform.xie.report", nonEmpty(state.PlatformXieArticleID, "manual"), map[string]any{"article_id": state.PlatformXieArticleID})
		}
	default:
		state.Message = "未知工作台动作"
	}
	s.renderPlatformWorkbench(w, r, user, state)
}

func (s *Server) handlePlatformNotice(w http.ResponseWriter, r *http.Request) {
	var notices []model.SystemNotice
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices); err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", notices)
}

func (s *Server) loadPlatformNotices() []model.SystemNotice {
	var notices []model.SystemNotice
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices); err != nil {
		return nil
	}
	sort.Slice(notices, func(i, j int) bool {
		return notices[i].CreatedAt.After(notices[j].CreatedAt)
	})
	if len(notices) > 5 {
		notices = notices[:5]
	}
	return notices
}

func (s *Server) loadPlatformAuditLogs(userID int64) []model.AuditLog {
	if userID <= 0 {
		return nil
	}
	var logs []model.AuditLog
	target := s.cfg.ContentURL + "/api/v1/system/audit-logs?limit=20&user_id=" + strconv.FormatInt(userID, 10)
	if err := s.getJSON(target, &logs); err != nil {
		return nil
	}
	filtered := make([]model.AuditLog, 0, len(logs))
	for _, entry := range logs {
		if strings.HasPrefix(strings.TrimSpace(entry.Action), "platform.") {
			filtered = append(filtered, entry)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	if len(filtered) > 8 {
		filtered = filtered[:8]
	}
	return filtered
}

func (s *Server) writePlatformAuditLog(user any, action, resource string, detail any) {
	userMap, _ := user.(map[string]any)
	entry := model.AuditLog{
		UserID:   userIDFromMap(user),
		Username: firstNonEmpty(mapString(userMap, "username"), mapString(userMap, "display_name")),
		Action:   strings.TrimSpace(action),
		Resource: strings.TrimSpace(resource),
	}
	if entry.UserID <= 0 || entry.Action == "" {
		return
	}
	if detail != nil {
		if raw, err := json.Marshal(detail); err == nil {
			entry.DetailJSON = string(raw)
		}
	}
	_, _ = s.client.R().
		SetBody(entry).
		Post(s.cfg.ContentURL + "/api/v1/system/audit-logs")
}

func (s *Server) runPlatformOCRWorkbench(r *http.Request, userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("未登录")
	}
	binding, err := s.getPlatformBinding(userID, "nlp")
	if err != nil {
		return "", fmt.Errorf("未绑定 NLP 服务")
	}
	filename, imageData, err := s.readLegacyNLPImagePayload(r)
	if err != nil {
		return "", err
	}
	results, code, msg, err := s.postLegacyNLPImage("ocr", filename, imageData, binding.SecretID, binding.SecretKey)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf(nonEmpty(msg, "OCR 识别失败"))
	}
	return summarizePlatformOCRResults(results), nil
}

func (s *Server) runPlatformImageWorkbench(r *http.Request, userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("未登录")
	}
	binding, err := s.getPlatformBinding(userID, "nlp")
	if err != nil {
		return "", fmt.Errorf("未绑定 NLP 服务")
	}
	filename, imageData, err := s.readLegacyNLPImagePayload(r)
	if err != nil {
		return "", err
	}
	results, code, msg, err := s.postLegacyNLPImage("image", filename, imageData, binding.SecretID, binding.SecretKey)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf(nonEmpty(msg, "图像识别失败"))
	}
	return summarizePlatformImageResults(results), nil
}

func (s *Server) runPlatformXieTitleWorkbench(r *http.Request, userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("未登录")
	}
	if _, err := s.getPlatformBinding(userID, "xie"); err != nil {
		return "", fmt.Errorf("未绑定写作服务")
	}
	articleID := strings.TrimSpace(firstNonEmpty(r.FormValue("article_id"), r.FormValue("articleId")))
	text := cleanXieText(nonEmpty(r.FormValue("text"), r.FormValue("content"), r.FormValue("summary")))
	if text == "" {
		text = s.articleTextForXie(articleID, map[string]string{
			"text":    r.FormValue("text"),
			"content": r.FormValue("content"),
			"summary": r.FormValue("summary"),
		})
	}
	if text == "" {
		return "", fmt.Errorf("文章内容不能为空")
	}
	return s.generateXieTitle(truncateRunes(stripHTMLTags(text), 2900)), nil
}

func (s *Server) runPlatformXieReportWorkbench(r *http.Request, userID int64) (string, string, error) {
	if userID <= 0 {
		return "", "", fmt.Errorf("未登录")
	}
	if _, err := s.getPlatformBinding(userID, "xie"); err != nil {
		return "", "", fmt.Errorf("未绑定写作服务")
	}
	articleID := strings.TrimSpace(firstNonEmpty(r.FormValue("article_id"), r.FormValue("articleId")))
	title := strings.TrimSpace(firstNonEmpty(r.FormValue("title"), r.FormValue("articleTitle"), r.FormValue("topic")))
	text := cleanXieText(nonEmpty(r.FormValue("text"), r.FormValue("content"), r.FormValue("summary")))
	params := map[string]string{
		"relatedword": firstNonEmpty(r.FormValue("relatedword"), r.FormValue("keyword")),
		"publishTime": r.FormValue("publishTime"),
		"title":       title,
	}
	if text == "" {
		text = s.articleTextForXie(articleID, map[string]string{
			"articleId":   articleID,
			"content":     r.FormValue("content"),
			"summary":     r.FormValue("summary"),
			"text":        r.FormValue("text"),
			"relatedword": params["relatedword"],
			"publishTime": params["publishTime"],
			"title":       title,
		})
	}
	if text == "" {
		return "", "", fmt.Errorf("文章内容不能为空")
	}
	if title == "" {
		title = s.generateXieTitle(truncateRunes(stripHTMLTags(text), 2900))
	}
	return title, composeXieReportText(title, text, params), nil
}

func summarizePlatformOCRResults(results any) string {
	list, ok := results.([]map[string]any)
	if !ok {
		return ""
	}
	lines := make([]string, 0, 8)
	for _, block := range list {
		rows, ok := block["data"].([]any)
		if !ok {
			continue
		}
		for _, row := range rows {
			entry, ok := row.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(legacyStringFromAny(entry["text"]))
			if text != "" {
				lines = append(lines, text)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func summarizePlatformImageResults(results any) string {
	payload, ok := results.(map[string]any)
	if !ok {
		return ""
	}
	rows, ok := payload["result"].([]any)
	if !ok {
		return ""
	}
	keywords := make([]string, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]any)
		if !ok {
			continue
		}
		keyword := strings.TrimSpace(legacyStringFromAny(entry["keyword"]))
		if keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	return strings.Join(keywords, "，")
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func publicOptionAnalysisText(option model.PublicOption, section string) string {
	switch strings.TrimSpace(section) {
	case "backanalysis":
		return option.BackAnalysis
	case "eventContext":
		return option.EventContext
	case "eventTrace":
		return option.EventTrace
	case "hotAnalysis":
		return option.HotAnalysis
	case "netizensAnalysis":
		return option.NetizensAnalysis
	case "statistics":
		return option.Statistics
	case "propagationAnalysis":
		return option.PropagationAnalysis
	case "thematicAnalysis":
		return option.ThematicAnalysis
	case "unscrambleContent":
		return option.UnscrambleContent
	case "popular_feelings_analys":
		return option.ContentAnalysis
	default:
		return option.EventContext
	}
}

func (s *Server) handlePlatformNLPBind(w http.ResponseWriter, r *http.Request, user any) {
	binding, ok := s.decodeLegacyBinding(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "读取请求失败", nil)
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	if binding.SecretID == "" || binding.SecretKey == "" {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "secretId和secretKey不能为空!", nil)
		return
	}
	binding.UserID = userID
	binding.Kind = "nlp"
	updated, err := s.putPlatformBinding(binding)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", updated)
}

func (s *Server) handlePlatformNLPImageCompat(w http.ResponseWriter, r *http.Request, user any, kind string) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	binding, err := s.getPlatformBinding(userID, "nlp")
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusOK, 424, "未绑定nlp服务", nil)
		return
	}
	filename, imageData, err := s.readLegacyNLPImagePayload(r)
	if err != nil {
		writeResultUtilJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	results, code, msg, err := s.postLegacyNLPImage(kind, filename, imageData, binding.SecretID, binding.SecretKey)
	if err != nil {
		writeResultUtilJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if code != http.StatusOK {
		writeResultUtilJSON(w, code, nonEmpty(msg, "识别失败"), nil)
		return
	}
	writeResultUtilJSON(w, http.StatusOK, "OK", results)
}

func (s *Server) readLegacyNLPImagePayload(r *http.Request) (string, []byte, error) {
	if filename, data, ok, err := s.readMultipartLegacyNLPImage(r); err != nil {
		return "", nil, err
	} else if ok {
		return filename, data, nil
	}
	if imageURL := firstNonEmpty(r.FormValue("imageUrl"), r.FormValue("image_url"), r.URL.Query().Get("imageUrl"), r.URL.Query().Get("image_url")); imageURL != "" {
		filename, _, data, err := s.fetchLegacyImage(imageURL)
		if err != nil {
			return "", nil, err
		}
		return filename, data, nil
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			imageURL := firstNonEmpty(legacyStringFromAny(payload["imageUrl"]), legacyStringFromAny(payload["image_url"]), legacyStringFromAny(payload["url"]))
			if imageURL != "" {
				filename, _, data, err := s.fetchLegacyImage(imageURL)
				if err != nil {
					return "", nil, err
				}
				return filename, data, nil
			}
		}
	}
	return "", nil, fmt.Errorf("imageUrl or image upload required")
}

func (s *Server) readMultipartLegacyNLPImage(r *http.Request) (string, []byte, bool, error) {
	for _, field := range []string{"images", "image", "file"} {
		file, header, err := r.FormFile(field)
		if err != nil {
			continue
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			return "", nil, false, err
		}
		filename := strings.TrimSpace(header.Filename)
		if filename == "" {
			filename = field
		}
		return filename, data, true, nil
	}
	if r.MultipartForm != nil {
		for _, headers := range r.MultipartForm.File {
			for _, header := range headers {
				file, err := header.Open()
				if err != nil {
					continue
				}
				defer file.Close()
				data, err := io.ReadAll(file)
				if err != nil {
					return "", nil, false, err
				}
				filename := strings.TrimSpace(header.Filename)
				if filename == "" {
					filename = "image"
				}
				return filename, data, true, nil
			}
		}
	}
	return "", nil, false, nil
}

func (s *Server) handlePlatformXieBind(w http.ResponseWriter, r *http.Request, user any) {
	binding, ok := s.decodeLegacyBinding(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "读取请求失败", nil)
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	if binding.SecretID == "" || binding.SecretKey == "" {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "secretId和secretKey不能为空!", nil)
		return
	}
	if sha1Hex(binding.SecretID) != strings.ToLower(binding.SecretKey) {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "secretId和secretKey不匹配!", nil)
		return
	}
	binding.UserID = userID
	binding.Kind = "xie"
	updated, err := s.putPlatformBinding(binding)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", updated)
}

func (s *Server) handlePlatformXieCheckBind(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	if _, err := s.getPlatformBinding(userID, "xie"); err != nil {
		writeLegacyStatusJSON(w, http.StatusOK, 424, "未绑定写作宝服务", nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, http.StatusOK, "OK", nil)
}

func (s *Server) handlePlatformXieTitle(w http.ResponseWriter, r *http.Request, user any, articleID string) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	if _, err := s.getPlatformBinding(userID, "xie"); err != nil {
		writeLegacyStatusJSON(w, http.StatusOK, 424, "未绑定写作宝服务", nil)
		return
	}
	copyWriting, ok := s.decodeCopyWriting(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "读取请求失败", nil)
		return
	}
	text := cleanXieText(nonEmpty(copyWriting.Params["text"], copyWriting.Params["content"], copyWriting.Params["summary"]))
	if text == "" {
		text = s.articleTextForXie(articleID, copyWriting.Params)
	}
	if text == "" {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, "文章内容为空", nil)
		return
	}
	copyWriting.PromptID = xieReportPromptID
	copyWriting.Params["text"] = truncateRunes(stripHTMLTags(text), 2900)
	title := s.generateXieTitle(copyWriting.Params["text"])
	writeLegacyStatusJSON(w, http.StatusOK, http.StatusOK, "OK", title)
}

func (s *Server) handlePlatformXieReportArticle(w http.ResponseWriter, r *http.Request, user any, articleID string) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	if _, err := s.getPlatformBinding(userID, "xie"); err != nil {
		writeLegacyStatusJSON(w, http.StatusOK, 424, "未绑定写作宝服务", nil)
		return
	}
	copyWriting, ok := s.decodeCopyWriting(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "读取请求失败", nil)
		return
	}
	text := cleanXieText(nonEmpty(copyWriting.Params["text"], copyWriting.Params["content"], copyWriting.Params["summary"]))
	if text == "" {
		text = s.articleTextForXie(articleID, copyWriting.Params)
	}
	if text == "" {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, "文章内容为空", nil)
		return
	}
	title := nonEmpty(copyWriting.Params["title"], copyWriting.Params["articleTitle"], copyWriting.Params["topic"])
	if title == "" {
		title = s.generateXieTitle(truncateRunes(stripHTMLTags(text), 2900))
	}
	s.streamXieReport(w, userID, articleID, title, text, copyWriting.Params)
}

func (s *Server) handlePlatformXieReportQuery(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	if _, err := s.getPlatformBinding(userID, "xie"); err != nil {
		writeLegacyStatusJSON(w, http.StatusOK, 424, "未绑定写作宝服务", nil)
		return
	}
	articleID := strings.TrimSpace(r.URL.Query().Get("articleId"))
	projectID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("projectId")), 10, 64)
	relatedWord := strings.TrimSpace(r.URL.Query().Get("relatedword"))
	publishTime := strings.TrimSpace(r.URL.Query().Get("publishTime"))
	title := strings.TrimSpace(r.URL.Query().Get("title"))
	text := s.articleTextForXie(articleID, map[string]string{
		"articleId":   articleID,
		"projectId":   strconv.FormatInt(projectID, 10),
		"relatedword": relatedWord,
		"publishTime": publishTime,
		"title":       title,
		"content":     "",
		"summary":     "",
	})
	if text == "" {
		text = nonEmpty(title, relatedWord, publishTime)
	}
	if title == "" {
		title = s.generateXieTitle(truncateRunes(stripHTMLTags(text), 2900))
	}
	s.streamXieReport(w, userID, articleID, title, text, map[string]string{
		"projectId":   strconv.FormatInt(projectID, 10),
		"relatedword": relatedWord,
		"publishTime": publishTime,
		"title":       title,
	})
}

func (s *Server) fetchLegacyImage(imageURL string) (string, string, []byte, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return "", "", nil, fmt.Errorf("image fetch failed")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", nil, err
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	filename := filepath.Base(mustParseURL(imageURL).Path)
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "image"
	}
	return filename, contentType, data, nil
}

func (s *Server) postLegacyNLPImage(kind, filename string, data []byte, secretID, secretKey string) (any, int, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("images", filename)
	if err != nil {
		return nil, 0, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, 0, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, 0, "", err
	}
	endpoint := s.cfg.NLPURL + "/api/v1/nlp/" + kind
	resp, err := s.client.R().
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetHeader("secret-id", secretID).
		SetHeader("secret-key", secretKey).
		SetBody(body.Bytes()).
		Post(endpoint)
	if err != nil {
		return nil, 0, "", err
	}
	var envelope struct {
		Code    int             `json:"code"`
		Msg     string          `json:"msg"`
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return nil, resp.StatusCode(), "", err
	}
	if envelope.Code != http.StatusOK {
		return nil, envelope.Code, envelope.Msg, nil
	}
	if kind == "ocr" {
		var results []map[string]any
		if len(envelope.Results) > 0 {
			if err := json.Unmarshal(envelope.Results, &results); err != nil {
				return nil, envelope.Code, envelope.Msg, err
			}
		}
		return results, envelope.Code, envelope.Msg, nil
	}
	var results map[string]any
	if len(envelope.Results) > 0 {
		if err := json.Unmarshal(envelope.Results, &results); err != nil {
			return nil, envelope.Code, envelope.Msg, err
		}
	}
	if results == nil {
		results = map[string]any{}
	}
	return results, envelope.Code, envelope.Msg, nil
}

func mustParseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		return &url.URL{}
	}
	return parsed
}

func (s *Server) handlePublicOptionEntry(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handlePublicOptionWorkbenchAction(w, r, user, 0)
		return
	}
	s.renderPublicOptionListPage(w, r, user, "")
}

func (s *Server) handlePublicOptionCompat(w http.ResponseWriter, r *http.Request, user any) {
	path := strings.TrimPrefix(r.URL.Path, "/publicoption/")
	switch {
	case path == "", path == "/":
		s.renderPublicOptionListPage(w, r, user, "")
	case path == "list":
		s.handlePublicOptionListJSON(w, r, user)
	case path == "getdatabyid":
		s.handlePublicOptionGetJSON(w, r, user)
	case path == "reportlist":
		s.renderPublicOptionReportListPage(w, r, user, "")
	case strings.HasPrefix(path, "reportdetail/"):
		id := strings.TrimPrefix(path, "reportdetail/")
		if r.Method == http.MethodPost {
			value, _ := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
			s.handlePublicOptionWorkbenchAction(w, r, user, value)
			return
		}
		s.renderPublicOptionReportDetailPage(w, r, user, id)
	case path == "updatedatabyid":
		s.handlePublicOptionUpdateJSON(w, r, user)
	case path == "addpublicoptiondata":
		s.handlePublicOptionCreateJSON(w, r, user)
	case path == "deletepublicoptioninfo":
		s.handlePublicOptionDeleteJSON(w, r, user)
	case path == "publicoptionreportlist":
		s.handlePublicOptionReportListJSON(w, r, user)
	case path == "backanalysis":
		s.renderPublicOptionAnalysisPage(w, r, user, "backanalysis")
	case path == "eventContext":
		s.renderPublicOptionAnalysisPage(w, r, user, "eventContext")
	case path == "eventTrace":
		s.renderPublicOptionAnalysisPage(w, r, user, "eventTrace")
	case path == "hotAnalysis":
		s.renderPublicOptionAnalysisPage(w, r, user, "hotAnalysis")
	case path == "netizensAnalysis":
		s.renderPublicOptionAnalysisPage(w, r, user, "netizensAnalysis")
	case path == "statistics":
		s.renderPublicOptionAnalysisPage(w, r, user, "statistics")
	case path == "propagationAnalysis":
		s.renderPublicOptionAnalysisPage(w, r, user, "propagationAnalysis")
	case path == "thematicAnalysis":
		s.renderPublicOptionAnalysisPage(w, r, user, "thematicAnalysis")
	case path == "unscrambleContent":
		s.renderPublicOptionAnalysisPage(w, r, user, "unscrambleContent")
	case path == "popular_feelings_analys":
		s.renderPublicOptionAnalysisPage(w, r, user, "popular_feelings_analys")
	case path == "loadInformation":
		s.handlePublicOptionLoadInformation(w, r, user)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handlePublicOptionWorkbenchAction(w http.ResponseWriter, r *http.Request, user any, selectedID int64) {
	_ = r.ParseForm()
	action := strings.TrimSpace(r.FormValue("action"))
	userID := userIDFromMap(user)
	if userID <= 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	redirectTo := "/publicoption"
	message := "操作失败"
	switch action {
	case "create":
		option, ok := s.decodePublicOptionRequest(r)
		if !ok {
			http.Redirect(w, r, "/publicoption?msg="+url.QueryEscape("任务参数无效"), http.StatusSeeOther)
			return
		}
		option.UserID = userID
		option.Status = 3
		option.DetailStatus = 3
		s.enrichPublicOption(&option)
		created, err := s.putPublicOptionCreate(option)
		if err == nil && created.ID > 0 {
			redirectTo = "/publicoption/reportdetail/" + strconv.FormatInt(created.ID, 10)
			message = "任务已创建"
		}
	case "update":
		option, ok := s.decodePublicOptionRequest(r)
		if !ok {
			http.Redirect(w, r, "/publicoption?msg="+url.QueryEscape("任务参数无效"), http.StatusSeeOther)
			return
		}
		if option.ID <= 0 {
			option.ID = selectedID
		}
		option.UserID = userID
		option.Status = 3
		option.DetailStatus = 3
		s.enrichPublicOption(&option)
		updated, err := s.putPublicOptionUpdate(option)
		if err == nil && updated.ID > 0 {
			redirectTo = "/publicoption/reportdetail/" + strconv.FormatInt(updated.ID, 10)
			message = "任务已更新"
		}
	case "delete":
		id := selectedID
		if id <= 0 {
			id, _ = strconv.ParseInt(firstNonEmpty(r.FormValue("id"), r.URL.Query().Get("id")), 10, 64)
		}
		if id > 0 && s.deletePublicOption(id) == nil {
			message = "任务已删除"
		}
	}
	http.Redirect(w, r, redirectTo+"?msg="+url.QueryEscape(message), http.StatusSeeOther)
}

func (s *Server) handlePublicOptionListJSON(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	userID := userIDFromMap(user)
	keyword := firstNonEmpty(r.FormValue("searchkeyword"), r.URL.Query().Get("searchkeyword"), r.URL.Query().Get("keyword"))
	options, err := s.getPublicOptions(userID, keyword)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyJSON(w, http.StatusOK, "ok", map[string]any{
		"list":      options,
		"pageCount": 1,
		"dataCount": len(options),
	})
}

func (s *Server) handlePublicOptionGetJSON(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	id := firstNonEmpty(r.FormValue("id"), r.URL.Query().Get("id"))
	option, err := s.getPublicOptionByID(id)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyJSON(w, http.StatusOK, "ok", map[string]any{"publicoption": option})
}

func (s *Server) handlePublicOptionReportListJSON(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	keyword := firstNonEmpty(r.FormValue("searchkeyword"), r.URL.Query().Get("searchkeyword"), r.URL.Query().Get("keyword"))
	options, err := s.getPublicOptions(userID, keyword)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyJSON(w, http.StatusOK, "ok", map[string]any{
		"list":      options,
		"pageCount": 1,
		"dataCount": len(options),
	})
}

func (s *Server) handlePublicOptionCreateJSON(w http.ResponseWriter, r *http.Request, user any) {
	option, ok := s.decodePublicOptionRequest(r)
	if !ok {
		writeLegacyJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	option.UserID = userIDFromMap(user)
	if option.UserID <= 0 {
		writeLegacyJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	option.Status = 3
	option.DetailStatus = 3
	s.enrichPublicOption(&option)
	created, err := s.putPublicOptionCreate(option)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyJSON(w, http.StatusOK, "ok", created)
}

func (s *Server) handlePublicOptionUpdateJSON(w http.ResponseWriter, r *http.Request, user any) {
	option, ok := s.decodePublicOptionRequest(r)
	if !ok {
		writeLegacyJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	option.UserID = userIDFromMap(user)
	if option.UserID <= 0 {
		writeLegacyJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	if option.ID <= 0 {
		option.ID, _ = strconv.ParseInt(firstNonEmpty(r.FormValue("id"), r.URL.Query().Get("id")), 10, 64)
	}
	if option.ID <= 0 {
		writeLegacyJSON(w, http.StatusBadRequest, "id required", nil)
		return
	}
	option.Status = 3
	option.DetailStatus = 3
	s.enrichPublicOption(&option)
	updated, err := s.putPublicOptionUpdate(option)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyJSON(w, http.StatusOK, "ok", updated)
}

func (s *Server) handlePublicOptionDeleteJSON(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	ids := firstNonEmpty(r.FormValue("Ids"), r.URL.Query().Get("Ids"), r.FormValue("ids"), r.URL.Query().Get("ids"))
	list := publicOptionIDs(ids)
	if len(list) == 0 {
		writeLegacyJSON(w, http.StatusBadRequest, "Ids required", nil)
		return
	}
	for _, id := range list {
		_ = s.deletePublicOption(id)
	}
	writeLegacyJSON(w, http.StatusOK, "ok", map[string]any{"deleted": true})
}

func (s *Server) handlePublicOptionLoadInformation(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	option, ok := s.decodePublicOptionRequest(r)
	if !ok {
		writeLegacyJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	option.Page = max(apiutilIntQuery(r, "page", option.Page), 1)
	payload, err := s.buildLoadInformation(option)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyJSON(w, http.StatusOK, "ok", payload)
}

func (s *Server) renderPublicOptionListPage(w http.ResponseWriter, r *http.Request, user any, message string) {
	s.renderPublicOptionWorkbench(w, r, user, 0, "", message)
}

func (s *Server) renderPublicOptionReportListPage(w http.ResponseWriter, r *http.Request, user any, message string) {
	s.renderPublicOptionListPage(w, r, user, message)
}

func (s *Server) renderPublicOptionReportDetailPage(w http.ResponseWriter, r *http.Request, user any, id string) {
	value, _ := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	s.renderPublicOptionWorkbench(w, r, user, value, "", "")
}

func (s *Server) renderPublicOptionAnalysisPage(w http.ResponseWriter, r *http.Request, user any, section string) {
	selectedID, _ := strconv.ParseInt(firstNonEmpty(r.URL.Query().Get("id"), r.FormValue("id")), 10, 64)
	s.renderPublicOptionWorkbench(w, r, user, selectedID, section, "")
}

func (s *Server) renderPublicOptionWorkbench(w http.ResponseWriter, r *http.Request, user any, selectedID int64, section string, message string) {
	userID := userIDFromMap(user)
	keyword := firstNonEmpty(r.URL.Query().Get("searchkeyword"), r.URL.Query().Get("keyword"))
	options, err := s.getPublicOptions(userID, keyword)
	if err != nil {
		_ = s.writeSimplePage(w, "publicoption/list", "事件分析", "<p>加载失败："+html.EscapeString(err.Error())+"</p>")
		return
	}
	selected := model.PublicOption{}
	if selectedID > 0 {
		for _, option := range options {
			if option.ID == selectedID {
				selected = option
				break
			}
		}
	}
	if selected.ID == 0 && len(options) > 0 {
		selected = options[0]
	}
	if selected.ID > 0 {
		if hydrated, err := s.getPublicOptionByID(strconv.FormatInt(selected.ID, 10)); err == nil && hydrated.ID > 0 {
			selected = hydrated
		}
	}
	if message == "" {
		message = r.URL.Query().Get("msg")
	}
	_ = s.render(w, "public_option_workbench", pageData{
		Title:         "事件分析工作台",
		User:          user,
		Message:       message,
		PublicOptions: options,
		PublicOption:  selected,
		Section:       section,
		FilterKeyword: keyword,
	})
}

func (s *Server) decodeLegacyBinding(r *http.Request) (model.PlatformBinding, bool) {
	var binding model.PlatformBinding
	if err := decodeBodyJSONOrForm(r, &binding); err != nil {
		return model.PlatformBinding{}, false
	}
	if binding.SecretID == "" {
		binding.SecretID = firstNonEmpty(r.FormValue("secretId"), r.FormValue("secret_id"), r.URL.Query().Get("secretId"), r.URL.Query().Get("secret_id"))
	}
	if binding.SecretKey == "" {
		binding.SecretKey = firstNonEmpty(r.FormValue("secretKey"), r.FormValue("secret_key"), r.URL.Query().Get("secretKey"), r.URL.Query().Get("secret_key"))
	}
	return binding, true
}

func (s *Server) decodeCopyWriting(r *http.Request) (legacyCopyWriting, bool) {
	var copyWriting legacyCopyWriting
	if err := decodeBodyJSONOrForm(r, &copyWriting); err != nil {
		return legacyCopyWriting{}, false
	}
	if copyWriting.Params == nil {
		copyWriting.Params = map[string]string{}
	}
	return copyWriting, true
}

func (s *Server) getPlatformBinding(userID int64, kind string) (model.PlatformBinding, error) {
	var binding model.PlatformBinding
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/platform/bindings/"+url.PathEscape(kind)+"?user_id="+strconv.FormatInt(userID, 10), &binding)
	return binding, err
}

func (s *Server) putPlatformBinding(binding model.PlatformBinding) (model.PlatformBinding, error) {
	var updated model.PlatformBinding
	resp, err := s.client.R().
		SetBody(binding).
		SetResult(&struct {
			Code int                   `json:"code"`
			Msg  string                `json:"message"`
			Data model.PlatformBinding `json:"data"`
		}{}).
		Post(s.cfg.ContentURL + "/api/v1/platform/bindings/" + url.PathEscape(binding.Kind))
	if err != nil {
		return model.PlatformBinding{}, err
	}
	if !resp.IsSuccess() {
		return model.PlatformBinding{}, fmt.Errorf(resp.Status())
	}
	var envelope struct {
		Code    int                   `json:"code"`
		Message string                `json:"message"`
		Data    model.PlatformBinding `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return model.PlatformBinding{}, err
	}
	updated = envelope.Data
	return updated, nil
}

func (s *Server) getPublicOptions(userID int64, keyword string) ([]model.PublicOption, error) {
	query := url.Values{}
	if userID > 0 {
		query.Set("user_id", strconv.FormatInt(userID, 10))
	}
	if keyword != "" {
		query.Set("keyword", keyword)
	}
	var options []model.PublicOption
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/public-options?"+query.Encode(), &options); err != nil {
		return nil, err
	}
	return options, nil
}

func (s *Server) getPublicOptionByID(id string) (model.PublicOption, error) {
	var option model.PublicOption
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/public-options/"+url.PathEscape(id), &option); err != nil {
		return model.PublicOption{}, err
	}
	return option, nil
}

func (s *Server) putPublicOptionCreate(option model.PublicOption) (model.PublicOption, error) {
	var created model.PublicOption
	resp, err := s.client.R().
		SetBody(option).
		SetResult(&struct {
			Code    int                `json:"code"`
			Message string             `json:"message"`
			Data    model.PublicOption `json:"data"`
		}{}).
		Post(s.cfg.ContentURL + "/api/v1/public-options")
	if err != nil {
		return model.PublicOption{}, err
	}
	if !resp.IsSuccess() {
		return model.PublicOption{}, fmt.Errorf(resp.Status())
	}
	var envelope struct {
		Code    int                `json:"code"`
		Message string             `json:"message"`
		Data    model.PublicOption `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return model.PublicOption{}, err
	}
	created = envelope.Data
	return created, nil
}

func (s *Server) putPublicOptionUpdate(option model.PublicOption) (model.PublicOption, error) {
	var updated model.PublicOption
	resp, err := s.client.R().
		SetBody(option).
		SetResult(&struct {
			Code    int                `json:"code"`
			Message string             `json:"message"`
			Data    model.PublicOption `json:"data"`
		}{}).
		Put(s.cfg.ContentURL + "/api/v1/public-options/" + strconv.FormatInt(option.ID, 10))
	if err != nil {
		return model.PublicOption{}, err
	}
	if !resp.IsSuccess() {
		return model.PublicOption{}, fmt.Errorf(resp.Status())
	}
	var envelope struct {
		Code    int                `json:"code"`
		Message string             `json:"message"`
		Data    model.PublicOption `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return model.PublicOption{}, err
	}
	updated = envelope.Data
	return updated, nil
}

func (s *Server) deletePublicOption(id int64) error {
	resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/public-options/" + strconv.FormatInt(id, 10))
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(resp.Status())
	}
	return nil
}

func (s *Server) enrichPublicOption(option *model.PublicOption) {
	option.EventStartTime = normalizeLegacyStartTime(option.EventStartTime)
	option.EventEndTime = normalizeLegacyEndTime(option.EventEndTime)
	info := s.buildPublicOptionAnalyses(*option)
	option.BackAnalysis = info["back_analysis"]
	option.EventContext = info["event_context"]
	option.EventTrace = info["event_trace"]
	option.HotAnalysis = info["hot_analysis"]
	option.NetizensAnalysis = info["netizens_analysis"]
	option.Statistics = info["statistics"]
	option.PropagationAnalysis = info["propagation_analysis"]
	option.ThematicAnalysis = info["thematic_analysis"]
	option.UnscrambleContent = info["unscramble_content"]
	option.ContentAnalysis = info["content_analysis"]
	if option.EmotionalIndex == "" {
		option.EmotionalIndex = "3"
	}
}

func (s *Server) buildLoadInformation(option model.PublicOption) (map[string]any, error) {
	articles, err := s.searchPublicOptionArticles(option)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(articles))
	for _, article := range articles {
		items = append(items, map[string]any{
			"_source": map[string]any{
				"title":          article.Title,
				"source_name":    nonEmpty(article.FromText, article.SourceType),
				"source_url":     nonEmpty(article.SourceURL, article.DetailURL),
				"publish_time":   nonEmpty(article.PublishTimeText, article.PublishTime),
				"emotionalIndex": articleEmotionIndex(article),
			},
		})
	}
	page := max(option.Page, 1)
	pageCount := 1
	if len(items) > 0 {
		pageCount = (len(items) + 9) / 10
	}
	return map[string]any{
		"page":       page,
		"page_count": pageCount,
		"data":       items,
		"dataCount":  len(items),
	}, nil
}

func (s *Server) searchPublicOptionArticles(option model.PublicOption) ([]model.Item, error) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(max(option.Page, 1)))
	query.Set("page_size", "10")
	query.Set("q", option.EventKeywords)
	query.Set("start", normalizeLegacyStartTime(option.EventStartTime))
	query.Set("end", normalizeLegacyEndTime(option.EventEndTime))
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result); err != nil {
		return nil, err
	}
	filtered := make([]model.Item, 0, len(result.Items))
	stopWords := splitLegacyLabels(option.EventStopWords)
	for _, item := range result.Items {
		if containsAny(item.Title+" "+item.Content, stopWords) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *Server) buildPublicOptionAnalyses(option model.PublicOption) map[string]string {
	items, _ := s.searchPublicOptionArticles(option)
	events := make([]map[string]any, 0, len(items))
	hot := make([]map[string]any, 0, len(items))
	figures := make([]map[string]any, 0, len(items))
	objects := make([]map[string]any, 0, len(items))
	media := make([]map[string]any, 0, len(items))
	for i, item := range items {
		events = append(events, map[string]any{
			"title":        item.Title,
			"publish_time": nonEmpty(item.PublishTimeText, item.PublishTime),
			"source_name":  nonEmpty(item.FromText, item.SourceType),
			"source_url":   nonEmpty(item.SourceURL, item.DetailURL),
		})
		if i < 8 {
			hot = append(hot, map[string]any{
				"topic":           item.Title,
				"publish_time":    nonEmpty(item.PublishTimeText, item.PublishTime),
				"source_name":     nonEmpty(item.FromText, item.SourceType),
				"source_url":      nonEmpty(item.SourceURL, item.DetailURL),
				"original_weight": len(item.Title) + len(item.Content),
			})
		}
		figures = append(figures, map[string]any{
			"name":       nonEmpty(item.FromText, item.SourceType),
			"author_url": nonEmpty(item.SourceURL, item.DetailURL),
			"content":    truncateRunes(item.Title+" "+item.Summary, 80),
			"value":      len(item.Title),
		})
		objects = append(objects, map[string]any{
			"title":        item.Title,
			"author":       nonEmpty(item.FromText, item.SourceType),
			"publish_time": nonEmpty(item.PublishTimeText, item.PublishTime),
			"hot":          len(item.Title) + len(item.Content),
		})
		media = append(media, map[string]any{
			"name":        nonEmpty(item.FromText, item.SourceType),
			"logo":        "",
			"abstract":    truncateRunes(item.Summary, 90),
			"source_name": nonEmpty(item.SourceType, item.FromText),
			"fans":        len(item.Title) * 10,
			"publishs":    len(item.Content),
		})
	}
	stats := map[string]any{
		"website":      bucketToNamedValues(bucketSourceCounts(items, []string{"flash", "headline"})),
		"weibo":        bucketToNamedValues(bucketSourceCounts(items, []string{"weibo"})),
		"social_media": bucketToNamedValues(bucketSourceCounts(items, []string{"wechat", "social"})),
		"wemedia":      bucketToNamedValues(bucketSourceCounts(items, nil)),
	}
	propagation := map[string]any{
		"media": media,
		"source": map[string]any{
			"all":     sourceAnalysisBuckets(items),
			"clinet":  sourceAnalysisBuckets(items),
			"website": sourceAnalysisBuckets(items),
			"BBS":     sourceAnalysisBuckets(items),
			"wechat":  sourceAnalysisBuckets(items),
			"weibo":   sourceAnalysisBuckets(items),
		},
	}
	netizens := map[string]any{
		"relation": map[string]any{
			"data":  []any{},
			"links": []any{},
		},
		"figure": figures,
		"object": objects,
	}
	back := map[string]any{
		"summary":   summarizeText(strings.Join(articleTitles(items), "，")),
		"keywords":  option.EventKeywords,
		"stopwords": option.EventStopWords,
	}
	trace := map[string]any{
		"backAnalysis": back,
		"eventContext": events,
	}
	thematic := map[string]any{
		"view":    hot[:minInt(len(hot), 5)],
		"media":   media[:minInt(len(media), 5)],
		"netizen": figures[:minInt(len(figures), 5)],
	}
	return map[string]string{
		"back_analysis":        mustJSONString(back),
		"event_context":        mustJSONString(events),
		"event_trace":          mustJSONString(trace),
		"hot_analysis":         mustJSONString(hot),
		"netizens_analysis":    mustJSONString(netizens),
		"statistics":           mustJSONString(stats),
		"propagation_analysis": mustJSONString(propagation),
		"thematic_analysis":    mustJSONString(thematic),
		"unscramble_content":   mustJSONString(map[string]any{"content": summarizeText(option.EventName + " " + option.EventKeywords)}),
		"content_analysis":     summarizeText(strings.Join(articleTitles(items), "；")),
	}
}

func (s *Server) articleTextForXie(articleID string, params map[string]string) string {
	if articleID != "" {
		var item model.Item
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/articles/"+url.PathEscape(articleID), &item); err == nil {
			return nonEmpty(item.Content, item.Summary, item.Title)
		}
	}
	return nonEmpty(params["text"], params["content"], params["summary"], params["title"], params["relatedword"], params["publishTime"])
}

func (s *Server) generateXieTitle(text string) string {
	text = truncateRunes(stripHTMLTags(text), 2900)
	if text == "" {
		return "自动生成标题"
	}
	var resp struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    model.NLPResponse `json:"data"`
	}
	if err := s.getJSONWithResult(s.cfg.NLPURL+"/api/v1/nlp/title", map[string]string{"text": text}, &resp); err == nil {
		if title := strings.TrimSpace(resp.Data.Title); title != "" {
			return title
		}
	}
	return truncateRunes(text, 24)
}

func composeXieReportText(title, text string, params map[string]string) string {
	return fmt.Sprintf(
		"标题：%s\n时间：%s\n关键词：%s\n\n%s",
		title,
		nonEmpty(params["publishTime"], time.Now().Format("2006-01-02 15:04:05")),
		params["relatedword"],
		truncateRunes(stripHTMLTags(text), 1000),
	)
}

func (s *Server) streamXieReport(w http.ResponseWriter, userID int64, articleID, title, text string, params map[string]string) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeLegacyJSON(w, http.StatusInternalServerError, "stream unsupported", nil)
		return
	}
	generated := fmt.Sprintf("标题：%s\n时间：%s\n关键词：%s\n\n%s", title, nonEmpty(params["publishTime"], time.Now().Format("2006-01-02 15:04:05")), params["relatedword"], truncateRunes(stripHTMLTags(text), 1000))
	writeSSEEvent(w, flusher, 1, "start", "start")
	runes := []rune(generated)
	for i, chunk := 0, 1; i < len(runes); i, chunk = i+12, chunk+1 {
		end := i + 12
		if end > len(runes) {
			end = len(runes)
		}
		writeSSEEvent(w, flusher, chunk+1, "message", mustJSONString(map[string]any{"data": string(runes[i:end])}))
	}
	writeSSEEvent(w, flusher, 999, "end", "end")
}

func (s *Server) getJSONWithResult(url string, body any, result any) error {
	resp, err := s.client.R().
		SetBody(body).
		SetResult(result).
		Post(url)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(resp.Status())
	}
	return nil
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, id int, event string, data string) {
	_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, event, data)
	flusher.Flush()
}

func decodeBodyJSONOrForm(r *http.Request, target any) error {
	if r.Body != nil {
		peek, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(peek))) > 0 {
			if err := json.Unmarshal(peek, target); err == nil {
				r.Body = io.NopCloser(strings.NewReader(string(peek)))
				return nil
			}
		}
		r.Body = io.NopCloser(strings.NewReader(string(peek)))
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	switch typed := target.(type) {
	case *model.PlatformBinding:
		typed.SecretID = firstNonEmpty(r.FormValue("secretId"), r.FormValue("secret_id"), typed.SecretID)
		typed.SecretKey = firstNonEmpty(r.FormValue("secretKey"), r.FormValue("secret_key"), typed.SecretKey)
		typed.Kind = firstNonEmpty(r.FormValue("kind"), r.URL.Query().Get("kind"), typed.Kind)
		if raw := firstNonEmpty(r.FormValue("userId"), r.FormValue("user_id"), r.URL.Query().Get("userId"), r.URL.Query().Get("user_id")); raw != "" {
			if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
				typed.UserID = value
			}
		}
	case *legacyCopyWriting:
		if typed.Params == nil {
			typed.Params = map[string]string{}
		}
		for _, key := range []string{"text", "title", "content", "summary", "relatedword", "publishTime", "articleId"} {
			if value := strings.TrimSpace(r.FormValue(key)); value != "" {
				typed.Params[key] = value
			}
		}
	case *model.PublicOption:
		typed.EventName = firstNonEmpty(r.FormValue("eventname"), r.FormValue("eventName"), typed.EventName)
		typed.EventKeywords = firstNonEmpty(r.FormValue("eventkeywords"), r.FormValue("eventKeywords"), typed.EventKeywords)
		typed.EventStopWords = firstNonEmpty(r.FormValue("eventstopwords"), r.FormValue("eventStopWords"), typed.EventStopWords)
		typed.EventStartTime = firstNonEmpty(r.FormValue("eventstarttime"), r.FormValue("eventStartTime"), typed.EventStartTime)
		typed.EventEndTime = firstNonEmpty(r.FormValue("eventendtime"), r.FormValue("eventEndTime"), typed.EventEndTime)
		if raw := firstNonEmpty(r.FormValue("id"), r.FormValue("publicoption_id"), r.URL.Query().Get("id")); raw != "" {
			if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
				typed.ID = value
			}
		}
		if raw := firstNonEmpty(r.FormValue("page"), r.URL.Query().Get("page")); raw != "" {
			if value, err := strconv.Atoi(raw); err == nil {
				typed.Page = value
			}
		}
		typed.EmotionalIndex = firstNonEmpty(r.FormValue("emotionalIndex"), r.FormValue("emotional_index"), typed.EmotionalIndex)
	default:
		return json.Unmarshal([]byte("{}"), target)
	}
	return nil
}

func (s *Server) getJSONWithForm(url string, form url.Values, result any) error {
	resp, err := s.client.R().
		SetBody(form.Encode()).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetResult(result).
		Post(url)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(resp.Status())
	}
	return nil
}

func (s *Server) decodePublicOptionRequest(r *http.Request) (model.PublicOption, bool) {
	var option model.PublicOption
	if err := decodeBodyJSONOrForm(r, &option); err != nil {
		return model.PublicOption{}, false
	}
	return option, true
}

func normalizeLegacyStartTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if strings.Contains(value, "00:00:00") {
		return value
	}
	return value + " 00:00:00"
}

func normalizeLegacyEndTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if strings.Contains(value, "23:59:59") {
		return value
	}
	return value + " 23:59:59"
}

func stripHTMLTags(text string) string {
	if text == "" {
		return ""
	}
	var out strings.Builder
	inTag := false
	for _, r := range text {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func truncateRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func cleanXieText(text string) string {
	return truncateRunes(stripHTMLTags(text), xieMaxInputLength)
}

func mustJSONString(value any) string {
	bytes, _ := json.Marshal(value)
	return string(bytes)
}

func articleTitles(items []model.Item) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Title)
	}
	return result
}

func bucketSourceCounts(items []model.Item, kinds []string) map[string]int {
	allowed := map[string]struct{}{}
	for _, kind := range kinds {
		allowed[strings.ToLower(kind)] = struct{}{}
	}
	counter := map[string]int{}
	for _, item := range items {
		source := strings.ToLower(strings.TrimSpace(item.SourceType))
		if len(allowed) > 0 {
			if _, ok := allowed[source]; !ok {
				continue
			}
		}
		counter[nonEmpty(item.FromText, item.SourceType)]++
	}
	return counter
}

func bucketToNamedValues(counter map[string]int) []map[string]any {
	out := make([]map[string]any, 0, len(counter))
	keys := make([]string, 0, len(counter))
	for key := range counter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, map[string]any{"name": key, "value": counter[key]})
	}
	return out
}

func sourceAnalysisBuckets(items []model.Item) []map[string]any {
	counter := bucketSourceCounts(items, nil)
	out := make([]map[string]any, 0, len(counter))
	keys := make([]string, 0, len(counter))
	for key := range counter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, map[string]any{
			"name":       key,
			"media_type": "article",
			"total":      counter[key],
			"negative":   counter[key] / 2,
		})
	}
	return out
}

func articleEmotionIndex(item model.Item) int {
	text := item.Title + " " + item.Content + " " + item.Summary
	text = strings.ToLower(text)
	if strings.Contains(text, "上涨") || strings.Contains(text, "利好") || strings.Contains(text, "增长") {
		return 1
	}
	if strings.Contains(text, "下跌") || strings.Contains(text, "风险") || strings.Contains(text, "暴跌") {
		return 3
	}
	return 2
}
