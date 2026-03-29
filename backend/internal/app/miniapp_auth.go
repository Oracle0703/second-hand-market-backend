package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"second-hand-market-backend/backend/internal/common"
)

const (
	miniProgramProviderWechat = "wechat"
	miniProgramProviderDouyin = "douyin"
)

type wechatCode2SessionResponse struct {
	OpenID  string  `json:"openid"`
	UnionID *string `json:"unionid"`
	ErrCode int     `json:"errcode"`
	ErrMsg  string  `json:"errmsg"`
}

type douyinCode2SessionRequest struct {
	AppID  string `json:"appid"`
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

type douyinCode2SessionResponse struct {
	Error   int     `json:"error"`
	ErrMsg  string  `json:"errmsg"`
	Message string  `json:"message"`
	OpenID  string  `json:"openid"`
	UnionID *string `json:"unionid"`
}

func miniProgramLoginConfigError(msg string) error {
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

func mapDouyinCode2SessionError(errCode int, msg string) error {
	switch errCode {
	case 10001, 2190002:
		return common.NewBizError(common.CodeInvalidArgument, "invalid douyin login code", http.StatusBadRequest)
	case 2150008:
		return common.ErrRateLimit
	default:
		if strings.TrimSpace(msg) != "" {
			return common.NewBizError(common.CodeUnauthorized, "douyin login failed", http.StatusUnauthorized)
		}
		return common.NewBizError(common.CodeUnauthorized, "douyin login failed", http.StatusUnauthorized)
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

func normalizeMiniProgramProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", miniProgramProviderWechat:
		return miniProgramProviderWechat, nil
	case miniProgramProviderDouyin, "tt":
		return miniProgramProviderDouyin, nil
	default:
		return "", common.NewBizError(common.CodeInvalidArgument, "unsupported miniapp provider", http.StatusBadRequest)
	}
}

func (s *Server) resolveMiniProgramIdentity(provider, code string) (string, string, *string, error) {
	normalizedProvider, err := normalizeMiniProgramProvider(provider)
	if err != nil {
		return "", "", nil, err
	}

	switch normalizedProvider {
	case miniProgramProviderWechat:
		openid, unionid, err := s.resolveWechatIdentity(code)
		return normalizedProvider, openid, unionid, err
	case miniProgramProviderDouyin:
		openid, unionid, err := s.resolveDouyinIdentity(code)
		return normalizedProvider, openid, unionid, err
	default:
		return "", "", nil, common.NewBizError(common.CodeInvalidArgument, "unsupported miniapp provider", http.StatusBadRequest)
	}
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
		return "", nil, miniProgramLoginConfigError("invalid BUYER_WECHAT_LOGIN_MODE")
	}

	if strings.TrimSpace(s.cfg.BuyerWechatAppID) == "" || strings.TrimSpace(s.cfg.BuyerWechatAppSecret) == "" {
		return "", nil, miniProgramLoginConfigError("wechat login is not configured: BUYER_WECHAT_APP_ID/BUYER_WECHAT_APP_SECRET required")
	}

	endpoint := strings.TrimSpace(s.cfg.BuyerWechatCode2SessionURL)
	if endpoint == "" {
		return "", nil, miniProgramLoginConfigError("wechat login is not configured: BUYER_WECHAT_CODE2SESSION_URL required")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", nil, miniProgramLoginConfigError("invalid BUYER_WECHAT_CODE2SESSION_URL")
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
		return "", nil, miniProgramLoginConfigError(fmt.Sprintf("wechat code2session request failed: %v", err))
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var payload wechatCode2SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", nil, miniProgramLoginConfigError("wechat code2session decode failed")
	}

	if payload.ErrCode != 0 {
		return "", nil, mapWechatCode2SessionError(payload.ErrCode)
	}

	oid := strings.TrimSpace(payload.OpenID)
	if oid == "" {
		return "", nil, miniProgramLoginConfigError("wechat code2session response missing openid")
	}
	return oid, trimOptional(payload.UnionID), nil
}

func (s *Server) resolveDouyinIdentity(code string) (string, *string, error) {
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return "", nil, common.ErrInvalidArgument
	}

	mode := strings.ToLower(strings.TrimSpace(s.cfg.BuyerDouyinLoginMode))
	if mode == "" || mode == "mock" {
		return "mock_tt_" + trimmedCode, nil, nil
	}
	if mode != "real" {
		return "", nil, miniProgramLoginConfigError("invalid BUYER_DOUYIN_LOGIN_MODE")
	}

	if strings.TrimSpace(s.cfg.BuyerDouyinAppID) == "" || strings.TrimSpace(s.cfg.BuyerDouyinAppSecret) == "" {
		return "", nil, miniProgramLoginConfigError("douyin login is not configured: BUYER_DOUYIN_APP_ID/BUYER_DOUYIN_APP_SECRET required")
	}

	endpoint := strings.TrimSpace(s.cfg.BuyerDouyinCode2SessionURL)
	if endpoint == "" {
		return "", nil, miniProgramLoginConfigError("douyin login is not configured: BUYER_DOUYIN_CODE2SESSION_URL required")
	}

	body, err := json.Marshal(douyinCode2SessionRequest{
		AppID:  strings.TrimSpace(s.cfg.BuyerDouyinAppID),
		Secret: strings.TrimSpace(s.cfg.BuyerDouyinAppSecret),
		Code:   trimmedCode,
	})
	if err != nil {
		return "", nil, miniProgramLoginConfigError("douyin code2session encode failed")
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, miniProgramLoginConfigError("invalid BUYER_DOUYIN_CODE2SESSION_URL")
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: s.cfg.BuyerDouyinHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, miniProgramLoginConfigError(fmt.Sprintf("douyin code2session request failed: %v", err))
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var payload douyinCode2SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", nil, miniProgramLoginConfigError("douyin code2session decode failed")
	}

	if payload.Error != 0 {
		return "", nil, mapDouyinCode2SessionError(payload.Error, payload.ErrMsg)
	}

	oid := strings.TrimSpace(payload.OpenID)
	if oid == "" {
		return "", nil, miniProgramLoginConfigError("douyin code2session response missing openid")
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

func (s *Server) wrapMiniProgramLoginError(err error) error {
	if err == nil {
		return nil
	}
	var bizErr *common.BizError
	if errors.As(err, &bizErr) {
		return bizErr
	}
	return miniProgramLoginConfigError("miniapp login failed")
}
