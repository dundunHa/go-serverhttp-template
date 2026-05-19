package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"

	logpkg "github.com/dundunHa/go-serverhttp-template/pkg/log"
)

// Config 整个服务的配置结构
type Config struct {
	AppEnv string `envconfig:"APP_ENV" default:"dev"`

	Server struct {
		Port int `envconfig:"PORT" default:"8080"`
	} `envconfig:"SERVER"`

	DB struct {
		DSN string `envconfig:"DSN" default:"postgres://postgres:postgres@localhost:5432/app?sslmode=disable"`
	} `envconfig:"DB"`

	Log logpkg.Config `envconfig:"LOG"`

	Cache CacheConfig `envconfig:"CACHE"`

	Auth AuthConfig `envconfig:"AUTH"`

	AppleIAP AppleIAPConfig `envconfig:"APPLE_IAP"`
}

// AppleIAPConfig 描述 Apple In-App Purchase 订阅相关配置。
//
// 解析后的环境变量统一以 APPLE_IAP_ 为前缀（由外层 `envconfig:"APPLE_IAP"`
// 与字段标签拼接得到），例如 APPLE_IAP_BUNDLE_ID、APPLE_IAP_PRIVATE_KEY。
//
// 安全契约（务必保持）：
//   - 携带凭证的字段（PrivateKey / P8Path / IssuerID / KeyID / BundleID / Products）
//     不得设置 envconfig default；缺失即视为未配置，由 payment 包判定 ErrNotConfigured。
//   - LoadConfig 仅做反序列化，不做业务校验；校验位置在
//     internal/service/payment/catalog.go，便于 HTTP 层干净地返回 503。
//   - 调用方不得日志输出 PrivateKey / P8Path 内容、Products JSON。
type AppleIAPConfig struct {
	BundleID   string `envconfig:"BUNDLE_ID"`
	IssuerID   string `envconfig:"ISSUER_ID"`
	KeyID      string `envconfig:"KEY_ID"`
	PrivateKey string `envconfig:"PRIVATE_KEY"`
	P8Path     string `envconfig:"P8_PATH"`
	Products   string `envconfig:"PRODUCTS"`

	EntitlementEnvironments string        `envconfig:"ENTITLEMENT_ENVIRONMENTS" default:"Production"`
	EnableSandboxFallback   bool          `envconfig:"ENABLE_SANDBOX_FALLBACK" default:"false"`
	WebhookMaxBodyBytes     int           `envconfig:"WEBHOOK_MAX_BODY_BYTES" default:"65536"`
	StoreRawPayloads        bool          `envconfig:"STORE_RAW_PAYLOADS" default:"false"`
	AppleAPITimeout         time.Duration `envconfig:"APPLE_API_TIMEOUT" default:"10s"`
}

// CacheConfig 定义 Redis 缓存相关配置
// 可根据需要调整字段名和类型
type CacheConfig struct {
	Addrs        []string      `envconfig:"ADDRS" default:"localhost:6379"`
	Password     string        `envconfig:"PASSWORD"`
	DB           int           `envconfig:"DB" default:"0"`
	PoolSize     int           `envconfig:"POOL_SIZE" default:"10"`
	DialTimeout  time.Duration `envconfig:"DIAL_TIMEOUT" default:"5s"`
	ReadTimeout  time.Duration `envconfig:"READ_TIMEOUT" default:"3s"`
	WriteTimeout time.Duration `envconfig:"WRITE_TIMEOUT" default:"3s"`
}

// AuthConfig 认证相关配置
type AuthConfig struct {
	Gmail GmailConfig `envconfig:"GMAIL"`
	Apple AppleConfig `envconfig:"APPLE"`
	JWT   JWTConfig   `envconfig:"JWT"`
}

// GmailConfig Gmail认证相关配置
type GmailConfig struct {
	ClientID string `envconfig:"CLIENT_ID"`
}

// AppleConfig Apple认证相关配置
type AppleConfig struct {
	ClientID        string        `envconfig:"CLIENT_ID"`
	JwksURL         string        `envconfig:"JWKS_URL" default:"https://appleid.apple.com/auth/keys"`
	RefreshInterval time.Duration `envconfig:"REFRESH_INTERVAL" default:"1h"`
}

// JWTConfig 本服务签发访问令牌所需配置
type JWTConfig struct {
	Secret         string        `envconfig:"SECRET" default:"dev-secret-change-me"`
	Issuer         string        `envconfig:"ISSUER" default:"go-serverhttp-template"`
	Audience       string        `envconfig:"AUDIENCE" default:"go-serverhttp-template-api"`
	AccessTokenTTL time.Duration `envconfig:"ACCESS_TOKEN_TTL" default:"15m"`
}

// LoadConfig 使用 envconfig 一次性处理所有字段
func LoadConfig() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("envconfig.Process: %w", err)
	}
	return &cfg, nil
}
