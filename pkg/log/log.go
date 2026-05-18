package log

import (
	"io"
	stdlog "log"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Level      string `envconfig:"LEVEL" default:"info"`
	Format     string `envconfig:"FORMAT" default:"console"`
	File       string `envconfig:"FILE"`
	MaxSize    int    `envconfig:"MAX_SIZE" default:"100"`
	MaxBackups int    `envconfig:"MAX_BACKUPS" default:"3"`
	MaxAge     int    `envconfig:"MAX_AGE" default:"28"`
	Compress   bool   `envconfig:"COMPRESS" default:"true"`
}

func InitLogger(cfg Config) {
	level, err := zerolog.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339

	writer := io.Writer(os.Stdout)
	if cfg.File != "" {
		writer = &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
	}
	if strings.EqualFold(cfg.Format, "console") {
		writer = zerolog.ConsoleWriter{Out: writer, TimeFormat: time.RFC3339}
	}

	log.Logger = zerolog.New(writer).With().Timestamp().Logger()
	stdlog.SetOutput(log.Logger)
}
