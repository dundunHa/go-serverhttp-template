package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"

	logpkg "go-serverhttp-template/pkg/log"
)

// Config 整个服务的配置结构
type Config struct {
	Server struct {
		Port int `envconfig:"PORT" default:"8080"`
	} `envconfig:"SERVER"`

	DB struct {
		Mysql struct {
			Addr string `envconfig:"ADDR" required:"true"`
		} `envconfig:"MYSQL"`
	} `envconfig:"DB"`

	Log logpkg.Config `envconfig:"LOG"`

	Cache CacheConfig `envconfig:"CACHE"`

	Auth AuthConfig `envconfig:"AUTH"`
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
	Apple AppleConfig `envconfig:"APPLE"`
}

// AppleConfig Apple认证相关配置
type AppleConfig struct {
	ClientID        string        `envconfig:"CLIENT_ID" required:"true"`
	JwksURL         string        `envconfig:"JWKS_URL" default:"https://appleid.apple.com/auth/keys"`
	RefreshInterval time.Duration `envconfig:"REFRESH_INTERVAL" default:"1h"`
}

// LoadConfig 使用 envconfig 一次性处理所有字段
func LoadConfig() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("SERVER", &cfg); err != nil {
		return nil, fmt.Errorf("envconfig.Process: %w", err)
	}
	return &cfg, nil
}
