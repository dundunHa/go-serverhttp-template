package cache

import "time"

// Config 定义 Redis 缓存相关配置
// 可根据需要调整字段名和类型
type Config struct {
	Addrs        []string      `mapstructure:"addrs" json:"addrs" yaml:"addrs"`
	Password     string        `mapstructure:"password" json:"password" yaml:"password"`
	DB           int           `mapstructure:"db" json:"db" yaml:"db"`
	PoolSize     int           `mapstructure:"pool_size" json:"pool_size" yaml:"pool_size"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout" json:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" json:"write_timeout" yaml:"write_timeout"`
}
