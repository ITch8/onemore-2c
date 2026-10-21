package temuops

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/trademind-ai/trademind/backend/internal/providers/platform/temu"
)

// SettingsReader loads decrypted settings for one group. Values must not be logged.
type SettingsReader func(ctx context.Context, tenantID int64, group string) (map[string]string, error)

// Gateway resolves Temu credentials and performs official router calls.
type Gateway struct {
	Config        temu.Config
	PlainSettings SettingsReader
	HTTP          *http.Client
}

func (g *Gateway) config() temu.Config {
	if g == nil {
		return temu.LoadConfig()
	}
	cfg := g.Config
	if cfg.Mode == "" {
		cfg.Mode = temu.ModeMock
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	return cfg
}

func (g *Gateway) resolve(ctx context.Context, tenantID int64) (temu.Config, error) {
	cfg := g.config()
	if g == nil || g.PlainSettings == nil {
		return cfg, nil
	}
	plain, err := g.PlainSettings(ctx, tenantID, "platform_temu")
	if err != nil {
		if cfg.Mode == temu.ModeLive {
			return cfg, invalid("读取 Temu 平台配置失败")
		}
		return cfg, nil
	}
	return mergeSettings(cfg, plain), nil
}

func mergeSettings(cfg temu.Config, plain map[string]string) temu.Config {
	if plain == nil {
		return cfg
	}
	if value := strings.TrimSpace(plain["app_key"]); value != "" {
		cfg.AppKey = value
	}
	if value := strings.TrimSpace(plain["app_secret"]); value != "" {
		cfg.AppSecret = value
	}
	if value := strings.TrimSpace(plain["access_token"]); value != "" {
		cfg.AccessToken = value
	}
	if value := strings.TrimSpace(plain["region"]); value != "" {
		cfg.Region = strings.ToLower(value)
	}
	if value := strings.TrimSpace(plain["api_base_url"]); value != "" {
		cfg.BaseURL = value
	}
	return cfg
}

func (g *Gateway) call(ctx context.Context, tenantID int64, apiType string, biz map[string]any) (*temu.CallResult, temu.Config, error) {
	cfg, err := g.resolve(ctx, tenantID)
	if err != nil {
		return nil, cfg, err
	}
	if cfg.Mode != temu.ModeLive {
		return nil, cfg, &Error{Kind: ErrLiveDisabled.Kind, Msg: "当前为演示模式，未调用 Temu"}
	}
	if strings.TrimSpace(apiType) == "" {
		return nil, cfg, &Error{Kind: ErrAPIGap.Kind, Msg: "Temu 开放接口未覆盖该动作"}
	}
	if !cfg.HasCredentials() {
		return nil, cfg, &Error{Kind: ErrCredentials.Kind, Msg: "正式接口缺少应用标识、应用密钥或访问令牌"}
	}
	client := temu.NewClient(cfg, g.HTTP)
	result, err := client.Call(ctx, apiType, biz)
	if err != nil {
		message := err.Error()
		if message == "" {
			message = "Temu 接口调用失败"
		}
		return result, cfg, invalid(message)
	}
	return result, cfg, nil
}
