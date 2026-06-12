package auth

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type wechatResult struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   any    `json:"data"`
}

func (s *Service) handleWechatGetQRCode(w http.ResponseWriter, r *http.Request) {
	challenge, err := s.createWechatChallenge(r.Context(), "login", 0)
	if err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	qr := model.WechatQRCode{
		SceneStr:  challenge.SceneStr,
		QRCodeURL: s.wechatQRCodeDataURL(challenge.SceneStr),
	}
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", qr)
}

func (s *Service) handleWechatGetBindQRCode(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeWechatResult(w, http.StatusForbidden, http.StatusForbidden, "unauthorized", nil)
		return
	}
	if err := s.ensureWechatAccountActive(user); err != nil {
		code := errCodeFromErr(err)
		writeWechatResult(w, http.StatusOK, code, err.Error(), nil)
		return
	}
	challenge, err := s.createWechatChallenge(r.Context(), "bind", user.ID)
	if err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	qr := model.WechatBindQRCode{
		SceneStr:  challenge.SceneStr,
		QRCodeURL: s.wechatQRCodeDataURL(challenge.SceneStr),
		Name:      s.cfg.WechatAccountName,
	}
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", qr)
}

func (s *Service) handleWechatCheckBind(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeWechatResult(w, http.StatusForbidden, http.StatusForbidden, "unauthorized", nil)
		return
	}
	if err := s.ensureWechatAccountActive(user); err != nil {
		code := errCodeFromErr(err)
		writeWechatResult(w, http.StatusOK, code, err.Error(), nil)
		return
	}
	if _, err := s.store.GetWechatBindingByUserID(r.Context(), user.ID); err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, "用户未绑定", nil)
		return
	}
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", nil)
}

func (s *Service) handleWechatWasBind(w http.ResponseWriter, r *http.Request) {
	sceneStr := strings.TrimSpace(r.URL.Query().Get("sceneStr"))
	if sceneStr == "" {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, "场景值不能为空", nil)
		return
	}
	challenge, err := s.store.GetWechatChallenge(r.Context(), sceneStr)
	if err != nil {
		writeWechatResult(w, http.StatusOK, 204, "未获取到用户操作", nil)
		return
	}
	if challenge.Purpose != "bind" {
		writeWechatResult(w, http.StatusOK, 500, "场景值不匹配", nil)
		return
	}
	if challenge.Status != "ready" || challenge.UserID <= 0 {
		writeWechatResult(w, http.StatusOK, 204, "未获取到用户操作", nil)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), challenge.UserID)
	if err != nil {
		writeWechatResult(w, http.StatusOK, 500, "用户不存在", nil)
		return
	}
	if err := s.ensureWechatAccountActive(user); err != nil {
		code := errCodeFromErr(err)
		writeWechatResult(w, http.StatusOK, code, err.Error(), nil)
		return
	}
	if err := s.store.DeleteWechatChallenge(r.Context(), sceneStr); err != nil {
		writeWechatResult(w, http.StatusOK, 500, err.Error(), nil)
		return
	}
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", nil)
}

func (s *Service) handleWechatCheckLogin(w http.ResponseWriter, r *http.Request) {
	sceneStr := strings.TrimSpace(r.URL.Query().Get("sceneStr"))
	if sceneStr == "" {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, "场景值不能为空", nil)
		return
	}
	challenge, err := s.store.GetWechatChallenge(r.Context(), sceneStr)
	if err != nil {
		writeWechatResult(w, http.StatusOK, 204, "未获取到用户操作", nil)
		return
	}
	if challenge.Purpose != "login" {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, "场景值不匹配", nil)
		return
	}
	if challenge.Status != "ready" || challenge.UserID <= 0 {
		writeWechatResult(w, http.StatusOK, 204, "未获取到用户操作", nil)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), challenge.UserID)
	if err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, "用户不存在", nil)
		return
	}
	if err := s.ensureWechatAccountActive(user); err != nil {
		code := errCodeFromErr(err)
		writeWechatResult(w, http.StatusOK, code, err.Error(), nil)
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, s.cfg.SessionTTL)
	if err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	_ = s.store.DeleteWechatChallenge(r.Context(), sceneStr)
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", map[string]any{
		"session_token": session.Token,
		"expires_at":    session.ExpiresAt,
		"user":          user,
	})
}

func (s *Service) handleWechatToken(w http.ResponseWriter, r *http.Request) {
	openID := strings.TrimSpace(r.URL.Query().Get("openid"))
	timestamp := strings.TrimSpace(r.URL.Query().Get("time"))
	ciphering := strings.TrimSpace(r.URL.Query().Get("ciphering"))
	if openID == "" || timestamp == "" || ciphering == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	parsedTime, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	sum := sha1.Sum([]byte(openID + strconv.FormatInt(parsedTime, 10) + s.cfg.WechatPrivateKey))
	if hex.EncodeToString(sum[:]) != strings.ToLower(ciphering) {
		w.WriteHeader(http.StatusOK)
		return
	}
	user, err := s.store.GetUserByOpenID(r.Context(), openID)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.ensureWechatAccountActive(user); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, s.cfg.SessionTTL)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(session.Token))
}

func (s *Service) handleWechatHandleSubscribe(w http.ResponseWriter, r *http.Request) {
	var event model.WechatEvent
	if err := decodeWechatEvent(r, &event); err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusBadRequest, err.Error(), nil)
		return
	}
	ok, err := s.applyWechatEvent(r.Context(), event, "login")
	if err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", ok)
}

func (s *Service) handleWechatHandleAuthorize(w http.ResponseWriter, r *http.Request) {
	var event model.WechatEvent
	if err := decodeWechatEvent(r, &event); err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusBadRequest, err.Error(), nil)
		return
	}
	ok, err := s.applyWechatEvent(r.Context(), event, "")
	if err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", ok)
}

func (s *Service) handleWechatHandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	sceneStr := strings.TrimSpace(r.URL.Query().Get("sceneStr"))
	if sceneStr == "" {
		writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", nil)
		return
	}
	_ = s.store.DeleteWechatChallenge(r.Context(), sceneStr)
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", nil)
}

func (s *Service) handleWechatMockScan(w http.ResponseWriter, r *http.Request) {
	var event model.WechatEvent
	event.SceneStr = strings.TrimSpace(r.URL.Query().Get("sceneStr"))
	event.OpenID = strings.TrimSpace(r.URL.Query().Get("openid"))
	if raw := strings.TrimSpace(r.URL.Query().Get("user_id")); raw != "" {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			event.UserID = value
		}
	}
	ok, err := s.applyWechatEvent(r.Context(), event, "")
	if err != nil {
		writeWechatResult(w, http.StatusOK, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeWechatResult(w, http.StatusOK, http.StatusOK, "OK", ok)
}

func (s *Service) applyWechatEvent(ctx context.Context, event model.WechatEvent, defaultPurpose string) (bool, error) {
	sceneStr := strings.TrimSpace(event.SceneStr)
	if sceneStr == "" {
		return false, errors.New("场景值不能为空")
	}
	challenge, err := s.store.GetWechatChallenge(ctx, sceneStr)
	if err != nil {
		return false, nil
	}
	if defaultPurpose != "" && challenge.Purpose != defaultPurpose && challenge.Purpose != "bind" {
		return false, nil
	}
	openID := strings.TrimSpace(event.OpenID)
	if challenge.Purpose == "bind" {
		userID := challenge.UserID
		if event.UserID > 0 {
			userID = event.UserID
		}
		if userID <= 0 {
			return false, nil
		}
		if openID == "" {
			openID = challenge.OpenID
		}
		if openID == "" {
			return false, nil
		}
		binding, err := s.store.UpsertWechatBinding(ctx, model.WechatBinding{UserID: userID, OpenID: openID})
		if err != nil {
			return false, err
		}
		now := time.Now().UTC()
		challenge.UserID = binding.UserID
		challenge.OpenID = binding.OpenID
		challenge.Status = "ready"
		challenge.CompletedAt = &now
		_, err = s.store.UpdateWechatChallenge(ctx, challenge)
		return err == nil, err
	}

	userID := event.UserID
	if userID <= 0 && openID != "" {
		if binding, err := s.store.GetWechatBindingByOpenID(ctx, openID); err == nil {
			userID = binding.UserID
		}
	}
	if userID <= 0 {
		if openID != "" {
			challenge.OpenID = openID
			_, _ = s.store.UpdateWechatChallenge(ctx, challenge)
		}
		return false, nil
	}
	now := time.Now().UTC()
	challenge.UserID = userID
	challenge.OpenID = openID
	challenge.Status = "ready"
	challenge.CompletedAt = &now
	_, err = s.store.UpdateWechatChallenge(ctx, challenge)
	return err == nil, err
}

func (s *Service) createWechatChallenge(ctx context.Context, purpose string, userID int64) (model.WechatChallenge, error) {
	sceneStr := "yuqing:" + strconv.FormatInt(time.Now().UTC().UnixNano(), 16)
	if purpose == "bind" && userID > 0 {
		sceneStr = "yuqing:" + strconv.FormatInt(userID, 16) + "#bind"
	}
	challenge := model.WechatChallenge{
		SceneStr:  sceneStr,
		Purpose:   purpose,
		UserID:    userID,
		Status:    "pending",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	return s.store.CreateWechatChallenge(ctx, challenge)
}

func (s *Service) wechatQRCodeDataURL(sceneStr string) string {
	svg := pseudoQRCodeSVG(s.cfg.AuthURL + "/api/v1/wechat/mock/scan?sceneStr=" + url.QueryEscape(sceneStr))
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func (s *Service) ensureWechatAccountActive(user model.User) error {
	if user.Status == 2 {
		return wechatAccountError{code: http.StatusInternalServerError, msg: "很抱歉，您的账号已被禁用，请联系管理员"}
	}
	if !user.TermOfValidity.IsZero() && user.TermOfValidity.Before(time.Now().UTC()) {
		return wechatAccountError{code: http.StatusGatewayTimeout, msg: "很抱歉，您的账号已过期，请联系管理员"}
	}
	return nil
}

type wechatAccountError struct {
	code int
	msg  string
}

func (e wechatAccountError) Error() string { return e.msg }

func errCodeFromErr(err error) int {
	var accountErr wechatAccountError
	if errors.As(err, &accountErr) {
		return accountErr.code
	}
	return http.StatusInternalServerError
}

func writeWechatResult(w http.ResponseWriter, httpStatus int, status int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(wechatResult{
		Status: status,
		Msg:    msg,
		Data:   data,
	})
}

func decodeWechatEvent(r *http.Request, target *model.WechatEvent) error {
	if r.Body == nil {
		return nil
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(target); err == nil {
		return nil
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	if err := r.ParseForm(); err != nil {
		return err
	}
	target.SceneStr = strings.TrimSpace(r.FormValue("sceneStr"))
	target.OpenID = strings.TrimSpace(r.FormValue("openid"))
	if raw := strings.TrimSpace(r.FormValue("user_id")); raw != "" {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			target.UserID = value
		}
	}
	if target.SceneStr == "" && target.OpenID == "" && target.UserID == 0 {
		return errors.New("invalid body")
	}
	return nil
}

func pseudoQRCodeSVG(content string) string {
	const size = 21
	const cell = 10
	const margin = 4
	sum := sha1.Sum([]byte(content))
	total := (size + margin*2) * cell
	var b strings.Builder
	b.Grow(4096)
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	b.WriteString(strconv.Itoa(total))
	b.WriteString(`" height="`)
	b.WriteString(strconv.Itoa(total))
	b.WriteString(`" viewBox="0 0 `)
	b.WriteString(strconv.Itoa(total))
	b.WriteString(` `)
	b.WriteString(strconv.Itoa(total))
	b.WriteString(`">`)
	b.WriteString(`<rect width="100%" height="100%" fill="#f4efe4"/>`)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			on := false
			if inFinder(x, y, 0, 0) || inFinder(x, y, size-7, 0) || inFinder(x, y, 0, size-7) {
				on = finderPattern(x, y, size)
			} else {
				idx := (x*31 + y*17 + int(sum[(x+y)%len(sum)])) % len(sum)
				on = (sum[idx]>>uint((x+y)%8))&1 == 1
			}
			if !on {
				continue
			}
			b.WriteString(`<rect x="`)
			b.WriteString(strconv.Itoa((x + margin) * cell))
			b.WriteString(`" y="`)
			b.WriteString(strconv.Itoa((y + margin) * cell))
			b.WriteString(`" width="`)
			b.WriteString(strconv.Itoa(cell))
			b.WriteString(`" height="`)
			b.WriteString(strconv.Itoa(cell))
			b.WriteString(`" fill="#16324f"/>`)
		}
	}
	b.WriteString(`<rect x="`)
	b.WriteString(strconv.Itoa((margin + 1) * cell))
	b.WriteString(`" y="`)
	b.WriteString(strconv.Itoa((margin + 1) * cell))
	b.WriteString(`" width="`)
	b.WriteString(strconv.Itoa((size + margin*2 - 2) * cell))
	b.WriteString(`" height="`)
	b.WriteString(strconv.Itoa((size + margin*2 - 2) * cell))
	b.WriteString(`" fill="none" stroke="#c9b99b" stroke-width="2"/>`)
	b.WriteString(`</svg>`)
	return b.String()
}

func inFinder(x, y, fx, fy int) bool {
	return x >= fx && x < fx+7 && y >= fy && y < fy+7
}

func finderPattern(x, y, size int) bool {
	switch {
	case x < 7 && y < 7:
		return finderCell(x, y)
	case x >= size-7 && y < 7:
		return finderCell(x-(size-7), y)
	case x < 7 && y >= size-7:
		return finderCell(x, y-(size-7))
	default:
		return false
	}
}

func finderCell(x, y int) bool {
	return x == 0 || x == 6 || y == 0 || y == 6 || (x >= 2 && x <= 4 && y >= 2 && y <= 4)
}
