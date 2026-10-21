package temu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

// CallResult is a trimmed Temu router response. It never includes request secrets.
type CallResult struct {
	Success      bool
	ErrorCode    string
	ErrorMessage string
	Result       map[string]any
}

// Client calls the official Temu Partner router over HTTPS with a timeout.
type Client struct {
	BaseURL           string
	AppKey            string
	AppSecret         string
	AccessToken       string
	AllowUnlistedHost bool
	HTTP              *http.Client
	Now               func() time.Time
}

func NewClient(cfg Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	if httpClient.Timeout <= 0 {
		httpClient.Timeout = cfg.Timeout
	}
	return &Client{
		BaseURL:           cfg.ResolvedBaseURL(),
		AppKey:            cfg.AppKey,
		AppSecret:         cfg.AppSecret,
		AccessToken:       cfg.AccessToken,
		AllowUnlistedHost: cfg.AllowUnlistedHost,
		HTTP:              httpClient,
		Now:               time.Now,
	}
}

func (c *Client) Call(ctx context.Context, apiType string, biz map[string]any) (*CallResult, error) {
	if c == nil || c.HTTP == nil {
		return nil, fmt.Errorf("temu client is not configured")
	}
	if err := ValidateBaseURL(c.BaseURL, c.AllowUnlistedHost); err != nil {
		return nil, err
	}
	apiType = strings.TrimSpace(apiType)
	if apiType == "" {
		return nil, fmt.Errorf("temu api type is empty")
	}
	if strings.TrimSpace(c.AppKey) == "" || strings.TrimSpace(c.AppSecret) == "" || strings.TrimSpace(c.AccessToken) == "" {
		return nil, fmt.Errorf("temu credentials are incomplete")
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	params := map[string]string{
		"type":         apiType,
		"app_key":      c.AppKey,
		"access_token": c.AccessToken,
		"timestamp":    strconv.FormatInt(now().Unix(), 10),
		"data_type":    "JSON",
	}
	for key, value := range biz {
		key = strings.TrimSpace(key)
		if key == "" || key == "sign" || key == "app_key" || key == "access_token" || key == "type" {
			continue
		}
		encoded, err := stringify(value)
		if err != nil {
			return nil, err
		}
		if encoded == "" {
			continue
		}
		params[key] = encoded
	}
	params["sign"] = Sign(c.AppSecret, params)
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("temu request encode failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("temu request build failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("temu request failed")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("temu response read failed")
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("temu response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("temu http status %d", resp.StatusCode)
	}
	return parseBody(raw)
}

func parseBody(raw []byte) (*CallResult, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("temu response is not json")
	}
	result := &CallResult{Result: map[string]any{}}
	if nested, ok := payload["result"].(map[string]any); ok {
		result.Result = nested
	}
	if success, ok := payload["success"].(bool); ok {
		result.Success = success
	}
	result.ErrorCode = firstScalar(payload, "errorCode", "error_code")
	result.ErrorMessage = firstScalar(payload, "errorMsg", "error_msg", "errorMessage")
	if result.ErrorCode == "1000000" || result.ErrorCode == "0" {
		result.Success = true
	}
	if !result.Success {
		if result.ErrorMessage == "" {
			result.ErrorMessage = "Temu 接口返回失败"
		}
		return result, fmt.Errorf("%s", result.ErrorMessage)
	}
	return result, nil
}

func firstScalar(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return strconv.FormatInt(int64(typed), 10)
		case json.Number:
			return typed.String()
		}
	}
	return ""
}

func stringify(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case json.Number:
		return typed.String(), nil
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("temu parameter encode failed")
		}
		return string(raw), nil
	}
}

// ValidateBaseURL restricts live calls to the official Temu router hosts.
// Loopback HTTP is allowed only when unlisted hosts are explicitly enabled,
// which production and staging ignore.
func ValidateBaseURL(raw string, allowUnlisted bool) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("temu base url is invalid")
	}
	host := strings.ToLower(parsed.Hostname())
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	switch parsed.Scheme {
	case "https":
	case "http":
		if !(allowUnlisted && loopback) {
			return fmt.Errorf("temu base url must use https")
		}
	default:
		return fmt.Errorf("temu base url must use https")
	}
	if ip := net.ParseIP(host); ip != nil && !loopback {
		return fmt.Errorf("temu base url host is not allowed")
	}
	switch host {
	case "openapi-b-us.temu.com", "openapi-b-eu.temu.com", "openapi-b-global.temu.com":
		return nil
	}
	if allowUnlisted && (loopback || strings.HasSuffix(host, ".temu.com")) {
		return nil
	}
	return fmt.Errorf("temu base url host is not allowed")
}
