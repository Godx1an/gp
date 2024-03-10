package routers

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"graduation_project/internal/middleware"
)

// SetupRouter 初始化路由
func SetupRouter() *gin.Engine {
	r := gin.Default()
	//r.Use(func(context *gin.Context) {
	//	// 记录调用方 IP 地址
	//	ip := context.ClientIP()
	//
	//	// 记录请求的路径和方法
	//	path := context.Request.RequestURI
	//	method := context.Request.Method
	//
	//	// 读取请求头
	//	headers := context.Request.Header
	//
	//	// 读取请求体
	//	body, _ := io.ReadAll(context.Request.Body)
	//	var jsonMap map[string]interface{}
	//	json.Unmarshal(body, &jsonMap)
	//	// 将请求体内容重新放入请求体中，以便后续处理程序读取
	//	context.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	//
	//	logrus.Infof("IP: %v; PATH: %v %v; HEADERS: %+v; BODY: %+v;\n", ip, method, path, headers, jsonMap)
	//
	//	context.Next()
	//})

	r.Use(cors.New(middleware.GetCorsConfig()))
	v1 := r.Group("/v1")
	// 初始化各个模块的路由
	InitUserRouters(v1)
	return r
}
