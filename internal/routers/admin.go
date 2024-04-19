package routers

import (
	"github.com/gin-gonic/gin"
	"graduation_project/internal/controllers"
	"graduation_project/internal/middleware"
)

func AdminRouters(r *gin.RouterGroup) {

	admin := r.Group("/admin")
	auth := admin.Group("")
	auth.Use(middleware.TokenVerAdmin())
	auth.POST("/modify_admin_nickname", controllers.ModifyAdminNickname)
	auth.POST("/modify_admin_pwd", controllers.ModifyAdminPwd)
	auth.POST("/bind_school", controllers.BindSchool)
	auth.POST("/admin_info", controllers.AdminInfo)
	auth.POST("/add_school", controllers.AddSchool)
	auth.POST("/add_item", controllers.AddItem)
	auth.POST("/admin_query_item", controllers.AdminQueryItem)
	auth.POST("/delete_item", controllers.DeleteItem)
	auth.POST("/add_school_item", controllers.AddSchoolItem)
	auth.POST("/query_school_item", controllers.QuerySchoolItem)
	auth.POST("/delete_school_item", controllers.DeleteSchoolItem)
	auth.POST("/modify_school_item", controllers.ModifySchoolItem)
	auth.POST("/query_user", controllers.QueryUser)
	auth.POST("/modify_item", controllers.ModifyItem)
	auth.POST("/query_school", controllers.QuerySchool)
	auth.POST("/delete_school", controllers.DeleteSchool)
	auth.POST("/admin_query_queue", controllers.AdminQueryQueue)
	auth.POST("/handle_queue", controllers.HandleQueue)
}
