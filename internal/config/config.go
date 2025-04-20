package config

import (
	"os"
	"strconv"

	logpkg "go-serverhttp-template/pkg/log"
)

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
