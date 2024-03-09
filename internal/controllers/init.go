package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
)

// DynamicResponseErr 自适应返回，error 是定义的就按定义的返回，否则按 500 返回并返回 err 的内容
func DynamicResponseErr(c *gin.Context, err error) {
	trueErr, ok := err.(code.MyCode)
	if ok {
		response.RespError(c, trueErr)
	} else {
		response.RespError(c, code.ServerErr)
	}
}
