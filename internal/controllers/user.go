package controllers

import (
	"errors"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/school"
	"github.com/Godx1an/gp_ent/pkg/ent_work/schoolfitnesstestitem"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/internal/handlers"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
	"time"
)

type account struct {
	OldPassword string `json:"oldPassword"`
	Pwd         string `json:"pwd"`
	NickName    string `json:"nickName"`
	School      string `json:"school"`
}

func ModifyNickname(c *gin.Context) {
	var (
		acc   account
		_user *ent_work.User
		err   error
	)
	err = c.ShouldBind(&acc)
	if err != nil {
		logrus.Errorf("modify bind with params err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}

	err = handlers.VerifyNickName(acc.NickName)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}
	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if _user.Nickname == acc.NickName {
			response.RespErrorWithMsg(c, code.InvalidParams, "新昵称与旧昵称一样")
			return errors.New("新昵称与旧昵称一样")
		}
		_, err = tx.User.UpdateOne(_user).SetNickname(acc.NickName).Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		return nil
	}); err != nil {
		return
	}
	response.RespSuccessWithMsg(c, nil, "修改成功")
	return
}

func ModifyPwd(c *gin.Context) {
	var (
		acc   account
		_user *ent_work.User
		err   error
	)
	err = c.ShouldBind(&acc)
	if err != nil {
		logrus.Errorf("modify bind with params err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}
	err = handlers.VerifyPwd(acc.Pwd)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if _user.Password != acc.OldPassword {
			response.RespErrorWithMsg(c, code.InvalidParams, "密码错误")
			return errors.New("密码错误")
		}
		if _user.Password == acc.Pwd {
			response.RespErrorWithMsg(c, code.InvalidParams, "新密码与原密码一样")
			return errors.New("新密码与原密码一样")
		}
		_, err = tx.User.UpdateOne(_user).SetPassword(acc.Pwd).Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		return nil
	}); err != nil {
		return
	}
	response.RespSuccessWithMsg(c, nil, "修改成功")
	return
}

func ModifySchool(c *gin.Context) {

	var (
		acc     account
		_user   *ent_work.User
		_school *ent_work.School
		err     error
	)

	err = c.ShouldBind(&acc)
	if err != nil {
		logrus.Errorf("modify bind with params err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}

	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_school, err = tx.School.Query().Where(school.Name(acc.School)).First(c)
		logrus.Info(_school)
		if err != nil {
			response.RespErrorWithMsg(c, code.InvalidParams, "系统暂无该学校")
			return errors.New("系统暂无该学校")
		}
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if acc.School != "" {
			if _user.School == acc.School {
				response.RespErrorWithMsg(c, code.InvalidParams, "新学校与旧学校一样")
				return errors.New("新学校与旧学校一样")
			}
		}
		if _user.NextUpdateTime.After(time.Now()) {
			response.RespErrorWithMsg(c, code.NotEnough, "您下次修改时间是"+_user.NextUpdateTime.Format("2006-01-02 15:04:05"))
			return errors.New("您下次修改时间是" + _user.NextUpdateTime.Format("2006-01-02 15:04:05"))
		}
		_, err = tx.User.UpdateOne(_user).SetSchool(acc.School).Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		dd, _ := time.ParseDuration("4320h")
		nextUpdateTime := time.Now().Add(dd)
		_, err = tx.User.UpdateOne(_user).SetNextUpdateTime(nextUpdateTime).Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		return nil
	}); err != nil {
		return
	}
	response.RespSuccessWithMsg(c, nil, "修改成功")
	return
}

func QueryItem(c *gin.Context) {

	var (
		acc     account
		_user   *ent_work.User
		_school *ent_work.School
		err     error
	)

	err = c.ShouldBind(&acc)
	if err != nil {
		logrus.Errorf("modify bind with params err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}

	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return err
		}
		_school, err = tx.School.Query().Where(school.Name(_user.School)).First(c)
		sftis, err := tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.DeletedAt(utils.ZeroTime)).Where(schoolfitnesstestitem.SchoolID(_school.ID)).All(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		response.RespSuccessWithMsg(c, sftis, "查询成功")
		return nil
	}); err != nil {
		return
	}
	return
}
