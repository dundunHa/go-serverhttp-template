package log

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config 定义日志系统所需配置
type Config struct {
	Environment string `toml:"environment" envconfig:"ENVIRONMENT" default:"development"`      // 环境: production 或 development
	LogLevel    string `toml:"level" envconfig:"LEVEL" default:"debug"`                        // 日志级别: trace/debug/info/warn/error/fatal/panic
	LogPath     string `toml:"path" envconfig:"PATH" default:"./logs"`                         // 日志输出目录
	AppName     string `toml:"app_name" envconfig:"APP_NAME" default:"go-serverhttp-template"` // 应用名称
	AppVersion  string `toml:"app_version" envconfig:"APP_VERSION" default:"0.0.1"`            // 应用版本
}

// ProcessInfoHook 在每条日志中添加进程 ID 和 goroutine 数量
type ProcessInfoHook struct{}

func (h ProcessInfoHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	e.Int("pid", os.Getpid()).
		Int("goroutines", runtime.NumGoroutine())
}

// InitLogger 根据配置初始化全局 Logger
func InitLogger(cfg Config) {

	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.SetGlobalLevel(parseLevel(cfg.LogLevel))

	var writer io.Writer
	if cfg.Environment == "production" {
		// 确保日志目录存在
		if err := os.MkdirAll(cfg.LogPath, os.ModePerm); err != nil {
			log.Fatal().Err(err).Msg("Failed to create log directory")
		}

		// 配置日志轮转
		lumberLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cfg.LogPath, cfg.AppName+".log"),
			MaxSize:    100,  // MB
			MaxBackups: 10,   // 最多保留旧文件数量
			MaxAge:     30,   // 天
			Compress:   true, // 是否压缩旧日志
		}
		writer = io.MultiWriter(os.Stdout, lumberLogger)
	} else {

		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		writer = consoleWriter
	}

	// 创建基础 Logger 并添加公共字段
	base := zerolog.New(writer).
		With().
		Timestamp().
		Str("app", cfg.AppName).
		Str("version", cfg.AppVersion).
		Caller().
		Logger()

	// 添加钩子
	log.Logger = base.Hook(ProcessInfoHook{})
}

// parseLevel 将字符串转换为 zerolog.Level
func parseLevel(level string) zerolog.Level {
	switch level {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}
