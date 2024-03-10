package controllers

import (
	"errors"
	"fmt"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/internal/db"
	"graduation_project/internal/handlers"
	"graduation_project/internal/myredis"
	"graduation_project/pkg"
	"graduation_project/pkg/jwt"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
	"net/http"
	"strconv"
	"time"
)

type LoginReq struct {
	Ticket        string `json:"ticket"`
	Phone         string `json:"phone"`
	Pwd           string `json:"pwd"`
	ConfirmPwd    string `json:"confirm_pwd"`
	Code          string `json:"code"`
	FastLoginCode string `json:"fast_login_code"`
	Way           string `json:"way"`
	AppType       string `json:"app_type"`
	JsCode        string `json:"js_code"`
	AppId         string `json:"app_id"`
	AutoLogin     bool   `json:"auto_login"`
	Email         string `json:"email"`
	AreaCode      string `json:"area_code"`
}

func Login(c *gin.Context) {
	var (
		req   LoginReq
		_user *ent_work.User
		err   error
	)
	err = c.ShouldBind(&req)
	if err != nil {
		logrus.Errorf("Login bind with params err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}
	switch req.Way {
	case WayPhonePwd:
		if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
			_user, err = tx.User.Query().Where(user.DeletedAt(utils.ZeroTime), user.Phone(req.Phone)).First(c)
			if ent_work.IsNotFound(err) {
				registerCode, err := myredis.Client.Get(c, fmt.Sprintf(RedisRegister, req.Phone)).Result()
				if err != nil {
					logrus.Errorf("redis获取出现错误：%v", err)
					response.RespErrorWithMsg(c, code.ServerErr, "未向该手机号发送注册短信")
					return err
				}
				if registerCode != req.Code {
					logrus.Error("code is incorrect")
					response.RespErrorWithMsg(c, FailCode, "验证码错误")
					return err
				}
				if !pkg.ValidatePassword(req.Pwd) {
					logrus.Error("pwd is invalid")
					response.RespErrorWithMsg(c, InvalidPwd, "密码不合法")
					return errors.New("密码不合法")
				}
				if req.Pwd != req.ConfirmPwd {
					logrus.Error("pwd and confirm pwd isn't same")
					response.RespErrorWithMsg(c, Inconsistent, "密码不一致")
					return errors.New("密码不一致")
				}
				// 没有找到，则进行默认注册，初始密码为设置密码
				_user, err = handlers.Register(c, tx, &handlers.RegisterOptions{
					Tx:           tx,
					Phone:        req.Phone,
					Password:     req.Pwd,
					IsEnterprise: false,
					Ticket:       req.Ticket,
				})
				if err != nil && errors.Is(err, code.FailGetInviteCode) {
					response.RespError(c, code.FailGetInviteCode)
					return err
				} else if err != nil {
					response.RespError(c, code.ServerErrDB)
					return err
				}
				return nil
			}
			if err != nil {
				response.RespErrorWithMsg(c, code.ServerErrDB, "数据库错误")
				return err
			}
			if _user.AreaCode != "+86" {
				logrus.Error("当前版本仅支持国内手机号使用")
				response.RespErrorWithMsg(c, code.ServerErr, "当前版本仅支持国内手机号使用")
				return errors.New("当前版本仅支持国内手机号使用")
			}
			if _user.Password != req.Pwd {
				logrus.Error("账号或密码错误")
				response.RespErrorWithMsg(c, FailPwd, "账号或密码错误")
				return code.ServerErr
			}
			return nil
		}); err != nil {
			return
		}
	default:
		response.RespErrorInvalidParams(c, errors.New("非法的登录方式"))
		return
	}

	resData := serializer.FrontendSerialize(_user)
	// 登录获得用户对象后，还要生成 jwt 给到前端
	generatedJwt, err := jwt.GenerateJwt(strconv.FormatInt(_user.ID, 10), req.AutoLogin)
	if err != nil {
		response.RespSuccessWithMsg(c, code.ServerErr, "获取jwt失败")
		return
	}
	cookie := http.Cookie{
		Name:    "token",
		Value:   "Bearer " + generatedJwt,
		Expires: time.Now().Add(jwt.TokenExpireDurationAutoLogin),
	}
	// 塞入 cookie
	logrus.Info("正在塞入cookie")
	http.SetCookie(c.Writer, &cookie)
	resData.(map[string]interface{})["token"] = generatedJwt
	response.RespSuccess(c, resData)
	err = db.DB.LoginRecord.Create().SetUserID(_user.ID).SetIP(c.ClientIP()).SetUa(c.GetHeader("User-Agent")).SetWay(req.Way).Exec(c)
	if err != nil {
		logrus.Error(err)
	}
	return

}
