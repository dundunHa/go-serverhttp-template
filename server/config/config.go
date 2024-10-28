package config

import (
	"os"
)

type Config struct {
	Server struct {
		Port int
	}

	MysqlConfig struct {
		Host     string
		Port     string
		User     string
		Password string
		DBName   string
	}
}

var conf *Config

func LoadConfig() *Config {
	if conf != nil {
		return conf
	}
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	conf = &Config{}
	conf.MysqlConfig.Host = dbHost
	conf.MysqlConfig.Port = dbPort
	conf.MysqlConfig.User = dbUser
	conf.MysqlConfig.Password = dbPassword
	conf.MysqlConfig.DBName = dbName

	return conf
}
