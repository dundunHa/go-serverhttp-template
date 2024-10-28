package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	logpkg "go-serverhttp-template/pkg/log"
)

const (
	// DefaultEnvPrefix 默认环境变量前缀（例如 APP_HTTP_PORT）。
	DefaultEnvPrefix = "APP"

	// DefaultConfigPath 默认配置文件路径（TOML）。
	DefaultConfigPath = "configs/config.toml"

	// ConfigPrefixEnvVar 控制环境变量前缀；为空表示不使用前缀（例如直接使用 MODE/HTTP_PORT）。
	// 该变量本身不使用前缀，避免“先有前缀才能读到前缀”的鸡生蛋问题。
	ConfigPrefixEnvVar = "CONFIG_PREFIX"

	// ConfigFileEnvVar 可选指定配置文件路径（TOML）；同样不使用前缀。
	ConfigFileEnvVar = "CONFIG_FILE"
)

type Mode string

const (
	ModeHTTP Mode = "http"
	ModeGRPC Mode = "grpc"
	ModeBoth Mode = "both"
)

type HTTPConfig struct {
	Port    int  `toml:"port"`
	LogBody bool `toml:"log_body"`
}

type GRPCConfig struct {
	Port int `toml:"port"`
}

type DBConfig struct {
	DSN             string        `toml:"dsn"`
	MaxOpenConns    int           `toml:"max_open_conns"`
	MaxIdleConns    int           `toml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `toml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `toml:"conn_max_idle_time"`
}

type CacheConfig struct {
	Addrs        []string      `toml:"addrs"`
	Password     string        `toml:"password"`
	DB           int           `toml:"db"`
	PoolSize     int           `toml:"pool_size"`
	DialTimeout  time.Duration `toml:"dial_timeout"`
	ReadTimeout  time.Duration `toml:"read_timeout"`
	WriteTimeout time.Duration `toml:"write_timeout"`
}

type AppleConfig struct {
	ClientID        string        `toml:"client_id"`
	JwksURL         string        `toml:"jwks_url"`
	RefreshInterval time.Duration `toml:"refresh_interval"`
}

type GmailConfig struct {
	ClientID string `toml:"client_id"`
}

type AuthConfig struct {
	Apple AppleConfig `toml:"apple"`
	Gmail GmailConfig `toml:"gmail"`
}

// Config 整个服务的配置结构：
// - 优先读取环境变量（统一前缀）
// - 其次读取 TOML 文件
type Config struct {
	Mode Mode `toml:"mode"`

	HTTP HTTPConfig `toml:"http"`
	GRPC GRPCConfig `toml:"grpc"`

	DB    DBConfig      `toml:"db"`
	Log   logpkg.Config `toml:"log"`
	Cache CacheConfig   `toml:"cache"`
	Auth  AuthConfig    `toml:"auth"`

	warnings []string `toml:"-"`
}

func DefaultConfig() Config {
	return Config{
		Mode: ModeHTTP,
		HTTP: HTTPConfig{
			Port:    8080,
			LogBody: false,
		},
		GRPC: GRPCConfig{
			Port: 9090,
		},
		DB: DBConfig{
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
		Log: logpkg.Config{
			Environment: "development",
			LogLevel:    "debug",
			LogPath:     "./logs",
			AppName:     "go-serverhttp-template",
			AppVersion:  "0.0.1",
		},
		Auth: AuthConfig{
			Apple: AppleConfig{
				JwksURL:         "https://appleid.apple.com/auth/keys",
				RefreshInterval: 1 * time.Hour,
			},
		},
		Cache: CacheConfig{
			Addrs:        []string{"localhost:6379"},
			DB:           0,
			PoolSize:     10,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		},
	}
}

func (c *Config) Warnings() []string {
	out := make([]string, 0, len(c.warnings))
	out = append(out, c.warnings...)
	return out
}

func (c *Config) addWarning(msg string) {
	c.warnings = append(c.warnings, msg)
}

func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	// 1) 读取 TOML（其次）
	prefix := resolveEnvPrefix()
	configFile := resolveConfigFile(prefix)
	if err := loadTOMLIfExists(&cfg, configFile); err != nil {
		return nil, err
	}

	// 2) 环境变量覆盖（优先）
	if err := cfg.applyEnvOverrides(prefix); err != nil {
		return nil, err
	}

	// 3) 启动期“必填项”告警（不阻塞启动）
	cfg.appendStartupWarnings(prefix)

	return &cfg, nil
}

func resolveEnvPrefix() string {
	if v, ok := os.LookupEnv(ConfigPrefixEnvVar); ok {
		// 允许显式设置为空字符串：表示不使用前缀
		return strings.TrimSpace(v)
	}
	return DefaultEnvPrefix
}

func resolveConfigFile(prefix string) string {
	if v, ok := os.LookupEnv(ConfigFileEnvVar); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	// 兼容：<prefix>_CONFIG_FILE
	if v, ok := lookup(prefix, "CONFIG_FILE"); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return DefaultConfigPath
}

func loadTOMLIfExists(cfg *Config, path string) error {
	if path == "" {
		return nil
	}
	abs := path
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			abs = filepath.Join(wd, path)
		}
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := toml.Unmarshal(b, cfg); err != nil {
		return fmt.Errorf("parse toml %q: %w", path, err)
	}
	return nil
}

func (c *Config) appendStartupWarnings(prefix string) {
	if c.DB.DSN == "" {
		c.addWarning(fmt.Sprintf("missing db.dsn (env %s); user APIs depending on DB will be disabled", envKey(prefix, "DB_DSN")))
	}
	if len(c.Cache.Addrs) == 0 {
		c.addWarning(fmt.Sprintf("missing cache.addrs (env %s); cache will be disabled", envKey(prefix, "CACHE_ADDRS")))
	}
	if c.Auth.Apple.ClientID == "" {
		c.addWarning(fmt.Sprintf("missing auth.apple.client_id (env %s); apple auth provider will be disabled", envKey(prefix, "AUTH_APPLE_CLIENT_ID")))
	}
	if c.Auth.Gmail.ClientID == "" {
		c.addWarning(fmt.Sprintf("missing auth.gmail.client_id (env %s); gmail auth provider will be disabled", envKey(prefix, "AUTH_GMAIL_CLIENT_ID")))
	}
}

func (c *Config) applyEnvOverrides(prefix string) error {
	// 新配置（统一前缀）
	if v, ok := lookup(prefix, "MODE"); ok {
		c.Mode = Mode(strings.ToLower(v))
	}
	if v, ok := lookup(prefix, "HTTP_PORT"); ok {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s_HTTP_PORT: %w", prefix, err)
		}
		c.HTTP.Port = p
	}
	if v, ok := lookup(prefix, "HTTP_LOG_BODY"); ok {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("%s_HTTP_LOG_BODY: %w", prefix, err)
		}
		c.HTTP.LogBody = b
	}
	if v, ok := lookup(prefix, "GRPC_PORT"); ok {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s_GRPC_PORT: %w", prefix, err)
		}
		c.GRPC.Port = p
	}
	if v, ok := lookup(prefix, "DB_DSN"); ok {
		c.DB.DSN = v
	}
	if v, ok := lookup(prefix, "DB_MAX_OPEN_CONNS"); ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s_DB_MAX_OPEN_CONNS: %w", prefix, err)
		}
		c.DB.MaxOpenConns = i
	}
	if v, ok := lookup(prefix, "DB_MAX_IDLE_CONNS"); ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s_DB_MAX_IDLE_CONNS: %w", prefix, err)
		}
		c.DB.MaxIdleConns = i
	}
	if v, ok := lookup(prefix, "DB_CONN_MAX_LIFETIME"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s_DB_CONN_MAX_LIFETIME: %w", prefix, err)
		}
		c.DB.ConnMaxLifetime = d
	}
	if v, ok := lookup(prefix, "DB_CONN_MAX_IDLE_TIME"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s_DB_CONN_MAX_IDLE_TIME: %w", prefix, err)
		}
		c.DB.ConnMaxIdleTime = d
	}

	// Log
	if v, ok := lookup(prefix, "LOG_ENVIRONMENT"); ok {
		c.Log.Environment = v
	}
	if v, ok := lookup(prefix, "LOG_LEVEL"); ok {
		c.Log.LogLevel = v
	}
	if v, ok := lookup(prefix, "LOG_PATH"); ok {
		c.Log.LogPath = v
	}
	if v, ok := lookup(prefix, "LOG_APP_NAME"); ok {
		c.Log.AppName = v
	}
	if v, ok := lookup(prefix, "LOG_APP_VERSION"); ok {
		c.Log.AppVersion = v
	}

	// Cache
	if v, ok := lookup(prefix, "CACHE_ADDRS"); ok {
		c.Cache.Addrs = splitComma(v)
	}
	if v, ok := lookup(prefix, "CACHE_PASSWORD"); ok {
		c.Cache.Password = v
	}
	if v, ok := lookup(prefix, "CACHE_DB"); ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s_CACHE_DB: %w", prefix, err)
		}
		c.Cache.DB = i
	}
	if v, ok := lookup(prefix, "CACHE_POOL_SIZE"); ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s_CACHE_POOL_SIZE: %w", prefix, err)
		}
		c.Cache.PoolSize = i
	}
	if v, ok := lookup(prefix, "CACHE_DIAL_TIMEOUT"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s_CACHE_DIAL_TIMEOUT: %w", prefix, err)
		}
		c.Cache.DialTimeout = d
	}
	if v, ok := lookup(prefix, "CACHE_READ_TIMEOUT"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s_CACHE_READ_TIMEOUT: %w", prefix, err)
		}
		c.Cache.ReadTimeout = d
	}
	if v, ok := lookup(prefix, "CACHE_WRITE_TIMEOUT"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s_CACHE_WRITE_TIMEOUT: %w", prefix, err)
		}
		c.Cache.WriteTimeout = d
	}

	// Auth
	if v, ok := lookup(prefix, "AUTH_APPLE_CLIENT_ID"); ok {
		c.Auth.Apple.ClientID = v
	}
	if v, ok := lookup(prefix, "AUTH_APPLE_JWKS_URL"); ok {
		c.Auth.Apple.JwksURL = v
	}
	if v, ok := lookup(prefix, "AUTH_APPLE_REFRESH_INTERVAL"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s_AUTH_APPLE_REFRESH_INTERVAL: %w", prefix, err)
		}
		c.Auth.Apple.RefreshInterval = d
	}
	if v, ok := lookup(prefix, "AUTH_GMAIL_CLIENT_ID"); ok {
		c.Auth.Gmail.ClientID = v
	}

	// 兼容旧变量（带警告）
	c.applyLegacyEnv(prefix)
	return nil
}

func (c *Config) applyLegacyEnv(prefix string) {
	// 旧: SERVER_PORT -> 新: APP_HTTP_PORT
	if c.HTTP.Port == 8080 {
		if v, ok := os.LookupEnv("SERVER_PORT"); ok {
			if p, err := strconv.Atoi(v); err == nil {
				c.HTTP.Port = p
				c.addWarning(fmt.Sprintf("deprecated env SERVER_PORT used; prefer %s", envKey(prefix, "HTTP_PORT")))
			}
		}
	}
	// 旧: LOG_LEVEL/LOG_PATH/APP_NAME/APP_VERSION（无统一前缀）
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok && c.Log.LogLevel == "debug" {
		c.Log.LogLevel = v
		c.addWarning(fmt.Sprintf("deprecated env LOG_LEVEL used; prefer %s", envKey(prefix, "LOG_LEVEL")))
	}
	if v, ok := os.LookupEnv("LOG_PATH"); ok && c.Log.LogPath == "./logs" {
		c.Log.LogPath = v
		c.addWarning(fmt.Sprintf("deprecated env LOG_PATH used; prefer %s", envKey(prefix, "LOG_PATH")))
	}
	if v, ok := os.LookupEnv("APP_NAME"); ok && c.Log.AppName == "go-serverhttp-template" {
		c.Log.AppName = v
		c.addWarning(fmt.Sprintf("deprecated env APP_NAME used; prefer %s", envKey(prefix, "LOG_APP_NAME")))
	}
	if v, ok := os.LookupEnv("APP_VERSION"); ok && c.Log.AppVersion == "0.0.1" {
		c.Log.AppVersion = v
		c.addWarning(fmt.Sprintf("deprecated env APP_VERSION used; prefer %s", envKey(prefix, "LOG_APP_VERSION")))
	}
	// 旧: SERVER_DB_MYSQL_ADDR -> 新: APP_DB_DSN
	if c.DB.DSN == "" {
		if v, ok := os.LookupEnv("SERVER_DB_MYSQL_ADDR"); ok {
			c.DB.DSN = v
			c.addWarning(fmt.Sprintf("deprecated env SERVER_DB_MYSQL_ADDR used; prefer %s", envKey(prefix, "DB_DSN")))
		}
	}
	// 旧: SERVER_AUTH_APPLE_CLIENT_ID -> 新: APP_AUTH_APPLE_CLIENT_ID
	if c.Auth.Apple.ClientID == "" {
		if v, ok := os.LookupEnv("SERVER_AUTH_APPLE_CLIENT_ID"); ok {
			c.Auth.Apple.ClientID = v
			c.addWarning(fmt.Sprintf("deprecated env SERVER_AUTH_APPLE_CLIENT_ID used; prefer %s", envKey(prefix, "AUTH_APPLE_CLIENT_ID")))
		}
	}
	// 旧: GMAIL_CLIENT_ID / COMFYUI_* 直接读取
	if c.Auth.Gmail.ClientID == "" {
		if v, ok := os.LookupEnv("GMAIL_CLIENT_ID"); ok {
			c.Auth.Gmail.ClientID = v
			c.addWarning(fmt.Sprintf("deprecated env GMAIL_CLIENT_ID used; prefer %s", envKey(prefix, "AUTH_GMAIL_CLIENT_ID")))
		}
	}
}

func lookup(prefix, key string) (string, bool) {
	if strings.TrimSpace(prefix) == "" {
		return os.LookupEnv(key)
	}
	return os.LookupEnv(prefix + "_" + key)
}

func envKey(prefix, key string) string {
	if strings.TrimSpace(prefix) == "" {
		return key
	}
	return prefix + "_" + key
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "t", "yes", "y", "on":
		return true, nil
	case "0", "false", "f", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", s)
	}
}
