package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"second-hand-market-backend/backend/internal/common"
)

type wechatCode2SessionResponse struct {
	OpenID  string  `json:"openid"`
	UnionID *string `json:"unionid"`
	ErrCode int     `json:"errcode"`
	ErrMsg  string  `json:"errmsg"`
}

func wechatLoginConfigError(msg string) error {
	return common.NewBizError(common.CodeInternal, msg, http.StatusInternalServerError)
}

func mapWechatCode2SessionError(errCode int) error {
	switch errCode {
	case 40029:
		return common.NewBizError(common.CodeInvalidArgument, "invalid wechat login code", http.StatusBadRequest)
	case 45011:
		return common.ErrRateLimit
	default:
		return common.NewBizError(common.CodeUnauthorized, "wechat login failed", http.StatusUnauthorized)
	}
}

func trimOptional(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	return &t
}

func (s *Server) resolveWechatIdentity(code string) (string, *string, error) {
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return "", nil, common.ErrInvalidArgument
	}

	mode := strings.ToLower(strings.TrimSpace(s.cfg.BuyerWechatLoginMode))
	if mode == "" || mode == "mock" {
		return "mock_wx_" + trimmedCode, nil, nil
	}
	if mode != "real" {
		return "", nil, wechatLoginConfigError("invalid BUYER_WECHAT_LOGIN_MODE")
	}

	if strings.TrimSpace(s.cfg.BuyerWechatAppID) == "" || strings.TrimSpace(s.cfg.BuyerWechatAppSecret) == "" {
		return "", nil, wechatLoginConfigError("wechat login is not configured: BUYER_WECHAT_APP_ID/BUYER_WECHAT_APP_SECRET required")
	}

	endpoint := strings.TrimSpace(s.cfg.BuyerWechatCode2SessionURL)
	if endpoint == "" {
		return "", nil, wechatLoginConfigError("wechat login is not configured: BUYER_WECHAT_CODE2SESSION_URL required")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", nil, wechatLoginConfigError("invalid BUYER_WECHAT_CODE2SESSION_URL")
	}
	q := u.Query()
	q.Set("appid", strings.TrimSpace(s.cfg.BuyerWechatAppID))
	q.Set("secret", strings.TrimSpace(s.cfg.BuyerWechatAppSecret))
	q.Set("js_code", trimmedCode)
	q.Set("grant_type", "authorization_code")
	u.RawQuery = q.Encode()

	client := &http.Client{Timeout: s.cfg.BuyerWechatHTTPTimeout}
	resp, err := client.Get(u.String())
	if err != nil {
		return "", nil, wechatLoginConfigError(fmt.Sprintf("wechat code2session request failed: %v", err))
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var payload wechatCode2SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", nil, wechatLoginConfigError("wechat code2session decode failed")
	}

	if payload.ErrCode != 0 {
		return "", nil, mapWechatCode2SessionError(payload.ErrCode)
	}

	oid := strings.TrimSpace(payload.OpenID)
	if oid == "" {
		return "", nil, wechatLoginConfigError("wechat code2session response missing openid")
	}
	return oid, trimOptional(payload.UnionID), nil
}

func setOptionalUpdate(updates map[string]interface{}, key string, value *string) {
	if value == nil {
		return
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		updates[key] = nil
		return
	}
	updates[key] = trimmed
}

func setOptionalModelField(dst **string, src *string) {
	if src == nil {
		return
	}
	trimmed := strings.TrimSpace(*src)
	if trimmed == "" {
		*dst = nil
		return
	}
	v := trimmed
	*dst = &v
}

func (s *Server) wrapWechatLoginError(err error) error {
	if err == nil {
		return nil
	}
	var bizErr *common.BizError
	if errors.As(err, &bizErr) {
		return bizErr
	}
	return wechatLoginConfigError("wechat login failed")
}
