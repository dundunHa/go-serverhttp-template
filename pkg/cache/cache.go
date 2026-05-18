package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis"
)

// Cache 核心类型
// 封装 redis.UniversalClient
type Cache struct {
	client redis.UniversalClient
}

// Default 全局默认实例，Init 后可直接使用下面的函数
var Default *Cache

// Init 用配置初始化 Default
func Init(cfg Config) {
	uopt := &redis.UniversalOptions{
		Addrs:        cfg.Addrs,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	cli := redis.NewUniversalClient(uopt)
	Default = &Cache{client: cli}
}

// Set 将任意对象 JSON 编码后存入
func Set(ctx context.Context, key string, val interface{}, ttl time.Duration) error {
	buf, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return Default.client.Set(key, buf, ttl).Err()
}

// Get[T] 从缓存取出并 JSON 解码到 T
func Get[T any](ctx context.Context, key string) (T, error) {
	var zero T
	buf, err := Default.client.Get(key).Bytes()
	if err != nil {
		return zero, err
	}
	if err := json.Unmarshal(buf, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

// Del 删除 key
func Del(ctx context.Context, key string) error {
	return Default.client.Del(key).Err()
}
