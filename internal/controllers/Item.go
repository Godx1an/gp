package controllers

import (
	"errors"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/admin"
	"github.com/Godx1an/gp_ent/pkg/ent_work/fitnesstestitem"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
	"time"
)

type ItemReq struct {
	Origin          string `json:"origin"`
	Item            string `json:"item"`
	MaxParticipants string `json:"maxParticipants"`
	Avg             string `json:"avg"`
}

func AddItem(c *gin.Context) {
	var (
		req    ItemReq
		_admin *ent_work.Admin
		_item  *ent_work.FitnessTestItem
		err    error
	)

	err = c.ShouldBind(&req)
	if err != nil {
		logrus.Errorf("Login bind with params err: %v", err)
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
		_admin, err = tx.Admin.Query().Where(admin.ID(UID), admin.DeletedAt(utils.ZeroTime)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return err
		}
		if _admin.ID != 1769198633205878784 {
			response.RespErrorWithMsg(c, code.AuthFailed, "非管理员")
			return errors.New("非管理员")
		}
		_, err = tx.FitnessTestItem.Query().Where(fitnesstestitem.Item(req.Item), fitnesstestitem.DeletedAt(utils.ZeroTime)).First(c)
		if err == nil {
			response.RespErrorWithMsg(c, code.FailHasRegister, "该项目已经存在")
			return errors.New("该项目已经存在")
		}
		_item, err = tx.FitnessTestItem.Query().Where(fitnesstestitem.Item(req.Item)).First(c)
		if err != nil {
			create := tx.FitnessTestItem.Create().
				SetItem(req.Item).
				SetCreatedBy(UID)
			_item, err = create.Save(c)
		} else {
			_item, err = tx.FitnessTestItem.UpdateOne(_item).SetDeletedAt(utils.ZeroTime).Save(c)
		}
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		response.RespSuccessWithMsg(c, _item, "添加成功")
		return nil
	}); err != nil {
		return
	}

	return
}

func AdminQueryItem(c *gin.Context) {
	var (
		//_admin *ent_work.Admin
		err error
	)

	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_, err = tx.Admin.Query().Where(admin.ID(UID), admin.DeletedAt(utils.ZeroTime)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return err
		}
		items, err := tx.FitnessTestItem.Query().Where(fitnesstestitem.DeletedAt(utils.ZeroTime)).All(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		response.RespSuccessWithMsg(c, items, "查询成功")
		return nil
	}); err != nil {
		return
	}

	return
}

func DeleteItem(c *gin.Context) {
	var (
		req    ItemReq
		_admin *ent_work.Admin
		_item  *ent_work.FitnessTestItem
		err    error
	)

	err = c.ShouldBind(&req)
	if err != nil {
		logrus.Errorf("Login bind with params err: %v", err)
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
		_admin, err = tx.Admin.Query().Where(admin.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return err
		}
		if _admin.ID != 1769198633205878784 {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return errors.New("权限校验失败")
		}
		_item, err = tx.FitnessTestItem.Query().Where(fitnesstestitem.Item(req.Item)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "找不到该项目")
			return errors.New("找不到该项目")
		}
		_, err = tx.FitnessTestItem.UpdateOne(_item).SetDeletedAt(time.Now()).Save(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.NotFound, "删除失败")
			return err
		}
		response.RespSuccess(c, "")
		return nil
	}); err != nil {
		return
	}
	return
}

// ModifyItem 修改项目
func ModifyItem(c *gin.Context) {
	var (
		req    ItemReq
		_admin *ent_work.Admin
		_item  *ent_work.FitnessTestItem
		err    error
	)

	err = c.ShouldBind(&req)
	if err != nil {
		logrus.Errorf("Login bind with params err: %v", err)
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
		_admin, err = tx.Admin.Query().Where(admin.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return err
		}
		if _admin.ID != 1769198633205878784 {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return errors.New("权限校验失败")
		}
		_item, err = tx.FitnessTestItem.Query().Where(fitnesstestitem.Item(req.Origin)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "找不到该项目")
			return errors.New("找不到该项目")
		}
		_, err = tx.FitnessTestItem.UpdateOne(_item).SetItem(req.Item).Save(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.NotFound, "修改失败")
			return err
		}
		response.RespSuccess(c, "")
		return nil
	}); err != nil {
		return
	}
	return
}
