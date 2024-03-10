package middleware

import (
	"github.com/gin-contrib/cors"
)

// GetCorsConfig 获取第三方宝解决跨域问题的初始化的配置，并自定义
func GetCorsConfig() cors.Config {
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowWildcard = true
	corsConfig.AllowOrigins = []string{"https://unicorn.org.cn", "http://localhost:*", "https://www.rosabi.cn", "http://192.168.31.223:*", "http://192.168.*"}
	addedHeaders := []string{"Access-Token", "Accept-Encoding", "X-CSRF-Token", "Authorization", "accept", "Cache-Control", "X-Requested-With", "Hmac-Key", "URI", "Hmac", "Hashed-Secret", "Secret"}
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders, addedHeaders...)
	corsConfig.AllowCredentials = true
	return corsConfig
}
