package controllers

import (
	"errors"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/admin"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
)

func UserInfo(c *gin.Context) {
	var (
		_user *ent_work.User
		err   error
	)

	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.ID(UID)).Where(user.DeletedAt(utils.ZeroTime)).First(c)
		if ent_work.IsNotFound(err) {
			response.RespError(c, code.ServerErrDB)
			return errors.New("数据库异常")
		}
		response.RespSuccessWithMsg(c, _user, "查找成功")
		return nil
	}); err != nil {
		return
	}

}

func AdminInfo(c *gin.Context) {
	var (
		_admin *ent_work.Admin
		err    error
	)

	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_admin, err = tx.Admin.Query().Where(admin.ID(UID)).Where(admin.DeletedAt(utils.ZeroTime)).First(c)
		if ent_work.IsNotFound(err) {
			response.RespError(c, code.ServerErrDB)
			return errors.New("数据库异常")
		}
		response.RespSuccessWithMsg(c, _admin, "查找成功")
		return nil
	}); err != nil {
		return
	}

}
