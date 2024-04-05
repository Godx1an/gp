package controllers

import (
	"errors"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/admin"
	"github.com/Godx1an/gp_ent/pkg/ent_work/fitnesstestitem"
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
	"strconv"
	"time"
)

// ModifyAdminNickname 更改管理员昵称
func ModifyAdminNickname(c *gin.Context) {
	var (
		acc    account
		_admin *ent_work.Admin
		err    error
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
		_admin, err = tx.Admin.Query().Where(admin.ID(UID)).First(c)
		if _admin.Nickname == acc.NickName {
			response.RespErrorWithMsg(c, code.InvalidParams, "新昵称与旧昵称一样")
			return errors.New("新昵称与旧昵称一样")
		}
		_, err = tx.Admin.UpdateOne(_admin).SetNickname(acc.NickName).Save(c)
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

func ModifyAdminPwd(c *gin.Context) {
	var (
		acc    account
		_admin *ent_work.Admin
		err    error
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
		_admin, err = tx.Admin.Query().Where(admin.ID(UID)).First(c)
		if _admin.Password == acc.Pwd {
			response.RespErrorWithMsg(c, code.InvalidParams, "新密码与原密码一样")
			return errors.New("新密码与原密码一样")
		}
		_, err = tx.Admin.UpdateOne(_admin).SetPassword(acc.Pwd).Save(c)
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

// BindSchool 绑定已存在的学校
func BindSchool(c *gin.Context) {

	var (
		acc     account
		_admin  *ent_work.Admin
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
		_school, err = tx.School.Query().Where(school.Name(acc.School), school.DeletedAt(utils.ZeroTime)).First(c)
		logrus.Info(_school)
		if err != nil {
			response.RespErrorWithMsg(c, code.InvalidParams, "系统暂无该学校")
			return errors.New("系统暂无该学校")
		}
		_admin, err = tx.Admin.Query().Where(admin.ID(UID), admin.DeletedAt(utils.ZeroTime)).First(c)
		_, err = tx.Admin.UpdateOne(_admin).SetSchool(acc.School).Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		return nil
	}); err != nil {
		return
	}
	response.RespSuccessWithMsg(c, nil, "绑定成功")
	return
}

// AddSchool 添加未存在的学校
func AddSchool(c *gin.Context) {
	var (
		acc    account
		_admin *ent_work.Admin
		err    error
	)
	err = c.ShouldBind(&acc)
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
		if _admin.School != "" {
			response.RespErrorWithMsg(c, code.FailHasRegister, "该用户已绑定过学校")
			return errors.New("该用户已绑定过学校")
		}
		_, err = tx.School.Query().Where(school.Name(acc.School), school.DeletedAt(utils.ZeroTime)).First(c)
		if err == nil {
			response.RespErrorWithMsg(c, code.InvalidParams, "该学校已存在")
			return errors.New("该学校已存在")
		}
		create := tx.School.Create().SetName(acc.School).SetCreatedBy(UID)
		_, err = create.Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		_, err = tx.Admin.UpdateOne(_admin).SetSchool(acc.School).Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}

		return nil
	}); err != nil {
		return
	}

	return
}

// AddSchoolItem 添加本校的项目
func AddSchoolItem(c *gin.Context) {
	var (
		item    ItemReq
		_admin  *ent_work.Admin
		_item   *ent_work.FitnessTestItem
		_school *ent_work.School
		_sfti   *ent_work.SchoolFitnessTestItem
		err     error
	)

	err = c.ShouldBind(&item)
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
	avg, err := strconv.Atoi(item.Avg)
	maxp, err := strconv.Atoi(item.MaxParticipants)
	print(avg)
	print(maxp)
	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_admin, err = tx.Admin.Query().Where(admin.ID(UID), admin.DeletedAt(utils.ZeroTime)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return err
		}
		_item, err = tx.FitnessTestItem.Query().Where(fitnesstestitem.Item(item.Item), fitnesstestitem.DeletedAt(utils.ZeroTime)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "未找到该项目")
			return err
		}
		_school, err = tx.School.Query().Where(school.Name(_admin.School), school.DeletedAt(utils.ZeroTime)).First(c)
		_, err = tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.School(_admin.School), schoolfitnesstestitem.Item(item.Item), schoolfitnesstestitem.DeletedAt(utils.ZeroTime)).First(c)
		if err == nil {
			response.RespErrorWithMsg(c, code.SourceExist, "本校已有该项目")
			return errors.New("本校已有该项目")
		}
		_sfti, err = tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.School(_admin.School), schoolfitnesstestitem.Item(item.Item)).First(c)
		if err != nil {
			create := tx.SchoolFitnessTestItem.Create().
				SetCreatedBy(UID).
				SetItemID(_item.ID).
				SetAvgTimePerPerson(avg).
				SetSchoolID(_school.ID).
				SetMaxParticipants(maxp).
				SetSchool(_admin.School).
				SetItem(item.Item)
			_sfti, err = create.Save(c)
		} else {
			_sfti, err = tx.SchoolFitnessTestItem.UpdateOne(_sfti).SetDeletedAt(utils.ZeroTime).SetAvgTimePerPerson(avg).SetMaxParticipants(maxp).Save(c)
		}
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		response.RespSuccessWithMsg(c, code.Success, "新增本校项目成功")
		return nil
	}); err != nil {
		return
	}
	return
}

// ModifySchoolItem 修改本校的项目
func ModifySchoolItem(c *gin.Context) {
	var (
		item   ItemReq
		_admin *ent_work.Admin
		_sfti  *ent_work.SchoolFitnessTestItem
		err    error
	)

	err = c.ShouldBind(&item)
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
	avg, err := strconv.Atoi(item.Avg)
	maxp, err := strconv.Atoi(item.MaxParticipants)

	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_admin, err = tx.Admin.Query().Where(admin.ID(UID), admin.DeletedAt(utils.ZeroTime)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.AuthFailed, "权限验证失败")
			return err
		}
		_sfti, err = tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.School(_admin.School), schoolfitnesstestitem.DeletedAt(utils.ZeroTime), schoolfitnesstestitem.Item(item.Item)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "未找到该项目")
			return err
		}
		_sfti, err = tx.SchoolFitnessTestItem.UpdateOne(_sfti).SetAvgTimePerPerson(avg).SetMaxParticipants(maxp).Save(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return err
		}
		response.RespSuccessWithMsg(c, _sfti, "修改成功")
		return nil
	}); err != nil {
		return
	}
	return
}

// QuerySchoolItem 修改本校的项目
func QuerySchoolItem(c *gin.Context) {
	var (
		item    ItemReq
		_admin  *ent_work.Admin
		_school *ent_work.School
		err     error
	)

	err = c.ShouldBind(&item)
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
		_school, err = tx.School.Query().Where(school.Name(_admin.School)).First(c)
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

// DeleteSchoolItem 删除本校的项目
func DeleteSchoolItem(c *gin.Context) {
	var (
		item   ItemReq
		_admin *ent_work.Admin
		_sfti  *ent_work.SchoolFitnessTestItem
		err    error
	)

	err = c.ShouldBind(&item)
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
		_sfti, err = tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.Item(item.Item), schoolfitnesstestitem.School(_admin.School), schoolfitnesstestitem.DeletedAt(utils.ZeroTime)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.NotFound, "本校无该项目")
			return errors.New("本校无该项目")
		}
		_, err = tx.SchoolFitnessTestItem.UpdateOne(_sfti).SetDeletedAt(time.Now()).Save(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "删除失败")
			return errors.New("删除失败")
		}
		response.RespSuccess(c, "")
		return nil
	}); err != nil {
		return
	}

	return
}

// QueryUSer 查询本校用户
func QueryUser(c *gin.Context) {
	var (
		_admin *ent_work.Admin
		err    error
		users  []*ent_work.User
	)
	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}
	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_admin, err = tx.Admin.Query().Where(admin.ID(UID)).First(c)
		if err != nil {
			response.RespError(c, code.ServerErrDB)
			return errors.New("数据库异常")
		}
		users, err = tx.User.Query().Where(user.School(_admin.School), user.DeletedAt(utils.ZeroTime)).All(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "查询失败")
			return errors.New("查询失败")
		}
		response.RespSuccess(c, users)
		return nil
	}); err != nil {
		return
	}

	return
}
