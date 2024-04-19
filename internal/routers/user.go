package routers

import (
	"github.com/gin-gonic/gin"
	"graduation_project/internal/controllers"
	"graduation_project/internal/middleware"
)

func UserRouters(r *gin.RouterGroup) {

	user := r.Group("/user")
	auth := user.Group("")
	auth.Use(middleware.TokenVer())
	auth.POST("/user_info", controllers.UserInfo)
	auth.POST("/modify_pwd", controllers.ModifyPwd)
	auth.POST("/modify_nickname", controllers.ModifyNickname)
	auth.POST("/modify_school", controllers.ModifySchool)
	auth.POST("/query_item", controllers.QueryItem)
	auth.POST("/choose_item", controllers.ChooseItem)
	auth.POST("/query_reservation", controllers.QueryReservation)
	auth.POST("/upload_avatar", controllers.UploadAvatar)
	auth.GET("/find_image", controllers.FindImage)
	auth.POST("/dequeue", controllers.Dequeue)
}
