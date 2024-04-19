package controllers

import (
	"errors"
	"fmt"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/internal/common"
	"graduation_project/internal/myredis"
	"graduation_project/internal/types"
	"graduation_project/utils/db_utils"
	"math/rand"
	"time"
)

const letterBytes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func GenerateRegisterCode(length int8) string {
	c := make([]byte, length)
	for i := range c {
		c[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(c)
}

type SendCodeReq struct {
	Email string `json:"email"`
	Code  string `json:"code"`
	Pwd   string `json:"pwd"`
}

func SendCode(c *gin.Context) {
	var (
		req SendCodeReq
		err error
	)
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	//检查是否存在该邮箱
	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_, err = tx.User.Query().Where(user.Email(req.Email)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.NotFound, "系统找不到该邮箱")
			return errors.New("系统找不到该邮箱")
		}
		return nil
	}); err != nil {
		return
	}

	logrus.Debugf("req: %+v", req)
	// 生成随机验证码
	randomCode := GenerateRegisterCode(types.RegisterCodeLength)

	// 保存到 myRedis
	rKey := fmt.Sprintf(types.RedisEmailCode, req.Email)
	err = myredis.Client.Set(c, rKey, randomCode, types.RedisEmailCodeExpireTime*time.Minute).Err()
	if err != nil {
		logrus.Errorf("failed to set email(%s) code(%s) to myRedis, err: %v", rKey, randomCode, err)
		response.RespError(c, code.ServerErrCache)
		return
	}
	// 发送验证码
	err = common.SendMail(req.Email, randomCode)
	if err != nil {
		response.RespError(c, code.ServerErrThirdPartyAPI)
		return
	}
	response.RespSuccess(c, "发送验证码成功")
}
func VerifyCode(c *gin.Context, email string, Code string) bool {
	var (
		err error
	)
	rKey := fmt.Sprintf(types.RedisEmailCode, email)
	rCode, err := myredis.Client.Get(c, rKey).Result()
	if !errors.Is(err, redis.Nil) {
		if err != nil {
			response.RespError(c, code.ServerErrCache)
			return false
		}
	} else {
		response.RespError(c, code.InvalidParams)
		return false
	}

	// 判断验证码是否正确
	if rCode != Code {
		logrus.Errorf("password error")
		response.RespError(c, code.InvalidKey)
		return false
	}
	return true
}
