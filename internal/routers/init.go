package routers

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"graduation_project/internal/middleware"
)

// SetupRouter 初始化路由
func SetupRouter() *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(middleware.GetCorsConfig()))
	v1 := r.Group("")
	// 初始化各个模块的路由
	UserLoginRouters(v1)
	AdminLoginRouters(v1)
	UserRouters(v1)
	AdminRouters(v1)
	return r
}
