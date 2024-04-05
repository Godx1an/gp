package controllers

import (
	"errors"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/admin"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/internal/handlers"
	"graduation_project/jwt"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
	"net/http"
	"strconv"
	"time"
)

type LoginReq struct {
	Phone           string `json:"phone"`
	Pwd             string `json:"pwd"`
	Code            string `json:"code"`
	ConfirmPassword string `json:"confirmPassword"`
}

func UserLogin(c *gin.Context) {
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
	err = handlers.VerifyPhone(req.Phone)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, err.Error())
		return
	}
	err = handlers.VerifyPwd(req.Pwd)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, err.Error())
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.Phone(req.Phone), user.Phone(req.Phone), user.DeletedAt(utils.ZeroTime)).First(c)
		if ent_work.IsNotFound(err) {
			create := tx.User.Create().
				SetPhone(req.Phone).
				SetPassword(req.Pwd).
				SetNickname("")
			_user, err = create.Save(c)
			if err != nil {
				response.RespError(c, code.ServerErrDB)
				return err
			}
			return nil
		}
		if req.Pwd != _user.Password {
			response.RespErrorWithMsg(c, FailPwd, "密码错误")
			return errors.New("密码错误")
		}
		return nil
	}); err != nil {
		return
	}

	resData := serializer.FrontendSerialize(_user)
	// 登录获得用户对象后，还要生成 jwt 给到前端
	generatedJwt, err := jwt.GenerateJwt(strconv.FormatInt(_user.ID, 10), true)
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
	if err != nil {
		logrus.Error(err)
	}
	return
}

func AdminLogin(c *gin.Context) {
	var (
		req    LoginReq
		_admin *ent_work.Admin
		err    error
	)
	err = c.ShouldBind(&req)
	if err != nil {
		logrus.Errorf("Login bind with params err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}
	err = handlers.VerifyPhone(req.Phone)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, err.Error())
		return
	}
	err = handlers.VerifyPwd(req.Pwd)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, err.Error())
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_admin, err = tx.Admin.Query().Where(admin.Phone(req.Phone), admin.DeletedAt(utils.ZeroTime)).First(c)
		if ent_work.IsNotFound(err) {
			create := tx.Admin.Create().
				SetPhone(req.Phone).
				SetPassword(req.Pwd).
				SetNickname("")
			_admin, err = create.Save(c)
			if err != nil {
				response.RespError(c, code.ServerErrDB)
				return err
			}
			return nil
		}
		if req.Pwd != _admin.Password {
			response.RespErrorWithMsg(c, FailPwd, "密码错误")
			return errors.New("密码错误")
		}
		return nil
	}); err != nil {
		return
	}

	resData := serializer.FrontendSerialize(_admin)
	// 登录获得用户对象后，还要生成 jwt 给到前端
	generatedJwt, err := jwt.GenerateJwt(strconv.FormatInt(_admin.ID, 10), true)
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
	if err != nil {
		logrus.Error(err)
	}
	return
}
