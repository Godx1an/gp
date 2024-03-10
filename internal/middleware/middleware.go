package middleware

import (
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/code_msgs"
	"github.com/stark-sim/serializer/response"
	"graduation_project/internal/db"
	"graduation_project/pkg/jwt"
	"strconv"
	"strings"
)

const (
	UserID = "user_id"
)

func TokenVer() gin.HandlerFunc {
	return func(c *gin.Context) {
		var err error
		var userID int64
		if token := c.GetHeader("Authorization"); token != "" {
			userID, err, _ = TokenToUserID(token)
			if err != nil {
				response.RespError(c, code.AuthFailed)
				c.Abort()
				return
			}
			_, err = db.DB.User.Query().Where(user.ID(userID)).First(c)
			if err != nil {
				response.RespError(c, code.AuthFailed)
				logrus.Error("数据库查询失败")
				c.Abort()
				return
			}
		} else if token, err = c.Cookie("token"); err != nil && token != "" {
			userID, err, _ = TokenToUserID(token)
			if err != nil {
				response.RespError(c, code.AuthFailed)
				c.Abort()
				return
			}
			_, err = db.DB.User.Query().Where(user.ID(userID)).First(c)
			if err != nil {
				response.RespError(c, code.AuthFailed)
				logrus.Error("数据库查询失败")
				c.Abort()
				return
			}
		} else {
			response.RespError(c, code.AuthFailed)
			c.Abort()
			return
		}
		c.Set(UserID, userID)
		c.Next()
		return
	}
}

func TokenToUserID(token string) (userID int64, err error, errMsg string) {
	if token == "" {
		return 0, code_msgs.ValidFail, "请求头中 auth 为空"
	}
	parts := strings.SplitN(token, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		return 0, code_msgs.ValidFail, "请求头 auth 格式错误"
	}
	mc, err := jwt.ParseToken(parts[1])
	if err != nil {
		return 0, code_msgs.ValidFail, "token 失效"
	}
	userID, err = strconv.ParseInt(mc.Info, 10, 64)
	if err != nil {
		return 0, code_msgs.Fail, "string 转 int64 失败"
	}
	return userID, nil, ""
}
