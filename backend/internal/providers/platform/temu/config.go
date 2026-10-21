package temu

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ModeMock   = "mock"
	ModeLive   = "live"
	PlatformID = "temu"
)

// APITypes are Temu Partner router method names. Empty means that call is an
// interface gap and must not be invented as a browser automation.
type APITypes struct {
	Ping       string
	Listing    string
	Price      string
	Stock      string
	Orders     string
	Label      string
	Ship       string
	AfterSales string
	Ads        string
	CSSend     string
}

// Config is deployment configuration. Secrets come from the environment or
// encrypted settings and must never be logged.
type Config struct {
	Mode              string
	LiveWrites        bool
	Region            string
	BaseURL           string
	AppKey            string
	AppSecret         string
	AccessToken       string
	Timeout           time.Duration
	AllowUnlistedHost bool
	AppEnv            string
	Types             APITypes
}

func LoadConfig() Config {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("TEMU_API_MODE")))
	if mode != ModeLive {
		mode = ModeMock
	}
	region := strings.ToLower(strings.TrimSpace(os.Getenv("TEMU_REGION")))
	if region == "" {
		region = "us"
	}
	timeout := 20 * time.Second
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("TEMU_HTTP_TIMEOUT_SECONDS"))); err == nil && n >= 5 && n <= 60 {
		timeout = time.Duration(n) * time.Second
	}
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	allow := strings.EqualFold(strings.TrimSpace(os.Getenv("TEMU_API_ALLOW_UNLISTED_HOST")), "true")
	if appEnv == "production" || appEnv == "staging" {
		allow = false
	}
	return Config{
		Mode:              mode,
		LiveWrites:        envBool("TEMU_LIVE_WRITES"),
		Region:            region,
		BaseURL:           strings.TrimSpace(os.Getenv("TEMU_API_BASE_URL")),
		AppKey:            strings.TrimSpace(os.Getenv("TEMU_APP_KEY")),
		AppSecret:         strings.TrimSpace(os.Getenv("TEMU_APP_SECRET")),
		AccessToken:       strings.TrimSpace(os.Getenv("TEMU_ACCESS_TOKEN")),
		Timeout:           timeout,
		AllowUnlistedHost: allow,
		AppEnv:            appEnv,
		Types: APITypes{
			Ping:       envOr("TEMU_API_TYPE_PING", "bg.open.accesstoken.info.get"),
			Listing:    envOr("TEMU_API_TYPE_LISTING", "bg.local.goods.add"),
			Price:      envOr("TEMU_API_TYPE_PRICE", "bg.local.goods.price.edit"),
			Stock:      envOr("TEMU_API_TYPE_STOCK", "bg.local.goods.stock.edit"),
			Orders:     envOr("TEMU_API_TYPE_ORDERS", "bg.order.list.v2.get"),
			Label:      envOr("TEMU_API_TYPE_LABEL", "bg.logistics.shipment.document.get"),
			Ship:       envOr("TEMU_API_TYPE_SHIP", "bg.logistics.shipment.confirm"),
			AfterSales: envOr("TEMU_API_TYPE_AFTERSALES", "bg.aftersales.refund.list.get"),
			Ads:        strings.TrimSpace(os.Getenv("TEMU_API_TYPE_ADS")),
			CSSend:     strings.TrimSpace(os.Getenv("TEMU_API_TYPE_CS_SEND")),
		},
	}
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func (c Config) HasCredentials() bool {
	return strings.TrimSpace(c.AppKey) != "" && strings.TrimSpace(c.AppSecret) != "" && strings.TrimSpace(c.AccessToken) != ""
}

// ResolvedBaseURL returns the explicit router URL or the official regional default.
func (c Config) ResolvedBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimSpace(c.BaseURL)
	}
	switch strings.ToLower(strings.TrimSpace(c.Region)) {
	case "eu":
		return "https://openapi-b-eu.temu.com/openapi/router"
	case "global":
		return "https://openapi-b-global.temu.com/openapi/router"
	default:
		return "https://openapi-b-us.temu.com/openapi/router"
	}
}
