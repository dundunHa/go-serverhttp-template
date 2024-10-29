package config

import (
	"os"
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
}

var conf *Config

func LoadConfig() *Config {
	if conf != nil {
		return conf
	}
	dbAddr := os.Getenv("DB_ADDR")

	conf = &Config{}
	conf.DB.Mysql.Addr = dbAddr

	return conf
}
