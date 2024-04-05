package routers

import (
	"github.com/gin-gonic/gin"
	"graduation_project/internal/controllers"
)

func UserLoginRouters(r *gin.RouterGroup) {

	r.POST("/user_login", controllers.UserLogin)

}

func AdminLoginRouters(r *gin.RouterGroup) {

	r.POST("/admin_login", controllers.AdminLogin)

}
