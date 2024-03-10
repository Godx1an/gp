package middleware

import (
	"fmt"
	cache "github.com/chenyahui/gin-cache"
	"github.com/chenyahui/gin-cache/persist"
	"github.com/gin-gonic/gin"
	"time"
)

func NewShortMemoryStore() gin.HandlerFunc {
	memoryStore := persist.NewMemoryStore(time.Second * 2)
	return cache.Cache(
		memoryStore,
		time.Second*2,
		cache.WithCacheStrategyByRequest(func(c *gin.Context) (bool, cache.Strategy) {
			return true, cache.Strategy{
				CacheKey:      fmt.Sprintf("%s%s%s", c.Request.RequestURI, c.GetHeader("Authorization"), c.GetHeader("Cookie")),
				CacheStore:    memoryStore,
				CacheDuration: time.Second * 2,
			}
		}),
	)
}
