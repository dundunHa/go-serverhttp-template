package config

import (
	"os"
	"strconv"
	"time"

	logpkg "go-serverhttp-template/pkg/log"
)

// CacheConfig 定义 Redis 缓存相关配置
// 可根据需要调整字段名和类型
type CacheConfig struct {
	Addrs        []string      `mapstructure:"addrs" json:"addrs" yaml:"addrs"`
	Password     string        `mapstructure:"password" json:"password" yaml:"password"`
	DB           int           `mapstructure:"db" json:"db" yaml:"db"`
	PoolSize     int           `mapstructure:"pool_size" json:"pool_size" yaml:"pool_size"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout" json:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" json:"write_timeout" yaml:"write_timeout"`
}

type Config struct {
	Server struct {
		Port int
	}

	DB struct {
		Mysql struct {
			Addr string
		}
	}

	Log logpkg.Config

	Cache CacheConfig `mapstructure:"cache" json:"cache" yaml:"cache"`
}

var conf *Config

func LoadConfig() *Config {
	if conf != nil {
		return conf
	}

	dbAddr := os.Getenv("DB_ADDR")
	portStr := os.Getenv("SERVER_PORT")
	if portStr == "" {
		portStr = "8080"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 8080
	}
	env := os.Getenv("GO_ENV")
	if env == "" {
		env = "development"
	}
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "debug"
	}
	logPath := os.Getenv("LOG_PATH")
	if logPath == "" {
		logPath = "./logs"
	}
	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = "go-serverhttp-template"
	}
	appVersion := os.Getenv("APP_VERSION")
	if appVersion == "" {
		appVersion = "0.0.1"
	}

	conf = &Config{}
	conf.DB.Mysql.Addr = dbAddr
	conf.Server.Port = port
	conf.Log = logpkg.Config{
		Environment: env,
		LogLevel:    logLevel,
		LogPath:     logPath,
		AppName:     appName,
		AppVersion:  appVersion,
	}

	return conf
}
