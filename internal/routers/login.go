package routers

import (
	"github.com/gin-gonic/gin"
	"graduation_project/internal/controllers"
)

func UserLoginRouters(r *gin.RouterGroup) {
	r.POST("/user_login", controllers.UserLogin)
	r.POST("/send_code", controllers.SendCode)
	r.POST("/reset_password", controllers.ResetPassword)
}

func AdminLoginRouters(r *gin.RouterGroup) {

	r.POST("/admin_login", controllers.AdminLogin)

}
