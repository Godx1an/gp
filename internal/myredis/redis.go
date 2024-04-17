package myredis

import (
	"github.com/go-redis/redis/v8"
	"time"
)

const (
	redisAddr = "%s:%d"
	cacheKey  = "my_cache_key"
)

var (
	Client         *redis.Client
	CacheExpire    time.Duration = 5 * time.Minute
	UserMissionSub *redis.PubSub
	UserWalletSub  *redis.PubSub
)

func init() {
	Client = redis.NewClient(&redis.Options{
		//Network:   "",
		Addr: "localhost:6379",
		//Dialer:    nil,
		//OnConnect: nil,
		//Username:  "",
		//Password:           configs.Conf.RedisConfig.Password,
		//DB:                 configs.Conf.RedisConfig.DB,
		Password: "",
		DB:       0,
		//MaxRetries:         0,
		//MinRetryBackoff:    0,
		//MaxRetryBackoff:    0,
		//DialTimeout:        0,
		//ReadTimeout:        0,
		//WriteTimeout:       0,
		//PoolFIFO:           false,
		//PoolSize:           10,
		//MinIdleConns:       1,
		//MaxConnAge:         0,
		//PoolTimeout:        0,
		//IdleTimeout:        0,
		//IdleCheckFrequency: 0,
		//TLSConfig:          nil,
		//Limiter:            nil,
	})
}
