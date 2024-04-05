package controllers

import (
	"fmt"
	"github.com/stark-sim/serializer/code"
)

// MsgCode 自定义规范错误返回信息
type MsgCode struct {
	Code int
	Msg  string
}

// 自定义返回错误

var (
	FAIL_OF_CONFIRM = MsgCode{3022, "确认失败"}
)

func (e MsgCode) Error() string {
	return fmt.Sprintf("Error %d: %s", e.Code, e.Msg)
}

const (
	RedisRegister string = "business_center:user.center:register:%s:string"
	WayPhonePwd   string = "phone_pwd"
)

// 返回 MyCode 类型

var (
	FailPwd      code.MyCode = 30007
	FailCode     code.MyCode = 40008
	Inconsistent code.MyCode = 40009
	InvalidPwd   code.MyCode = 40010
)
