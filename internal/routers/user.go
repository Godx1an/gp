package routers

import (
	"github.com/gin-gonic/gin"
	"graduation_project/internal/controllers"
	"graduation_project/internal/middleware"
)

func InitUserRouters(r *gin.RouterGroup) {
	r.POST("/login", controllers.Login)

	auth := r.Group("")
	auth.Use(middleware.TokenVer())
}
