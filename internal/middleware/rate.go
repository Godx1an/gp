package middleware

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Limiter 限流每秒访问一次
var Limiter = rate.NewLimiter(5, 1)

func RateMiddleware() func(c *gin.Context) {
	return func(c *gin.Context) {
		if Limiter.Allow() {
			c.Next()
		} else {
			c.JSON(429, gin.H{
				"code": 429,
				"msg":  "Too many requests",
			})
			c.Abort()
		}
	}
}
