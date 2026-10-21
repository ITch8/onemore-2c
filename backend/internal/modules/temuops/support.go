package temuops

import (
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

var (
	idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,128}$`)
	currencyPattern    = regexp.MustCompile(`^[A-Z]{3}$`)
	trackingPattern    = regexp.MustCompile(`^[A-Za-z0-9-]{4,64}$`)
)

func normalizeKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if !idempotencyPattern.MatchString(key) {
		return "", invalid("请提供 8 到 128 位的幂等键")
	}
	return key, nil
}

func normalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = "USD"
	}
	if !currencyPattern.MatchString(value) {
		return "", invalid("币种需为 3 位字母")
	}
	return value, nil
}

func validateSourceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if utf8.RuneCountInString(raw) > 2048 {
		return "", invalid("来源链接过长")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", invalid("来源链接无效")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", invalid("来源链接只支持 http 或 https")
	}
	return raw, nil
}

func clampText(value string, max int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}

func mustJSON(value any) datatypes.JSON {
	if value == nil {
		return datatypes.JSON([]byte("null"))
	}
	raw, err := json.Marshal(redactValue(value))
	if err != nil {
		return datatypes.JSON([]byte(`{"truncated":true}`))
	}
	if len(raw) > 16*1024 {
		raw = raw[:16*1024]
	}
	return datatypes.JSON(raw)
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSecretKey(key) {
				out[key] = "***"
				continue
			}
			out[key] = redactValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactValue(item)
		}
		return out
	default:
		return value
	}
}

func isSecretKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "app_secret", "appsecret", "access_token", "accesstoken", "refresh_token", "refreshtoken", "sign", "secret", "password", "cookie", "token":
		return true
	default:
		return strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "password")
	}
}

func minorFromMajor(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(math.Round(typed * 100))
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0
		}
		return int64(math.Round(parsed * 100))
	case string:
		parsed, err := json.Number(strings.TrimSpace(typed)).Float64()
		if err != nil {
			return 0
		}
		return int64(math.Round(parsed * 100))
	case int:
		return int64(typed) * 100
	case int64:
		return typed * 100
	default:
		return 0
	}
}

func stringField(item map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return strings.TrimSpace(strings.TrimRight(strings.TrimRight(
				jsonNumber(typed), "0"), "."))
		}
	}
	return ""
}

func jsonNumber(value float64) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func optionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return nil, invalid("编号格式无效")
	}
	return &id, nil
}

func checkMoney(value int64) error {
	if value < 0 || value > 1_000_000_000_000 {
		return invalid("金额超出允许范围")
	}
	return nil
}
