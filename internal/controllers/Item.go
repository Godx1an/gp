package controllers

import (
	"errors"
	"fmt"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/admin"
	"github.com/Godx1an/gp_ent/pkg/ent_work/fitnesstestitem"
	"github.com/Godx1an/gp_ent/pkg/ent_work/schoolfitnesstestitem"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/internal/myredis"
	"graduation_project/internal/types"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
	"strconv"
	"time"
)

type ItemReq struct {
	Id              string   `json:"id"`
	Origin          string   `json:"origin"`
	Item            string   `json:"item"`
	MaxParticipants string   `json:"maxParticipants"`
	Avg             string   `json:"avg"`
	Ids             []string `json:"ids"`
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

func ChooseItem(c *gin.Context) {
	var (
		req   ItemReq
		_user *ent_work.User
		err   error
	)
	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	//将UID转为string类型
	userID = strconv.FormatInt(UID, 10)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}
	err = c.ShouldBind(&req)
	if err != nil {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}

	rKey := fmt.Sprintf(types.RedisItem, req.Item)

	values := myredis.Client.LRange(c, rKey, 0, -1).Val()
	exist := false
	for _, value := range values {
		if value == userID {
			exist = true
			break
		}
	}
	if exist {
		response.RespErrorWithMsg(c, code.SourceExist, "您已选择该项目")
		return
	}
	err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}

		err = myredis.Client.RPush(c, rKey, _user.ID).Err()
		if err != nil {
			logrus.Errorf("failed to push user ID (%d) to Redis list (%s), err: %v", _user.ID, rKey, err)
			response.RespError(c, code.ServerErrCache)
			return errors.New("Redis list push failed")
		}
		return nil
	})
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	length := myredis.Client.LLen(c, rKey).Val()
	response.RespSuccess(c, length)
}

// QueryReservation 查询预约项目
func QueryReservation(c *gin.Context) {
	var (
		err error
	)
	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}
	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err := tx.User.Query().Where(user.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		sftis, err := tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.School(_user.School), schoolfitnesstestitem.DeletedAt(utils.ZeroTime)).All(c)
		item, err := queryEveryItem(c, _user, sftis)
		if err != nil {
			return err
		}
		response.RespSuccessWithMsg(c, item, "查询成功")
		return nil
	}); err != nil {
		return
	}

	return
}

type AppointmentInfo struct {
	ItemName   string // 项目名称
	QueueIndex int    // 预约排队位置
	ItemTime   int
	ItemMax    int
	ItemId     string
}

func queryEveryItem(c *gin.Context, user *ent_work.User, sftis []*ent_work.SchoolFitnessTestItem) ([]AppointmentInfo, error) {
	// 创建一个空的 AppointmentInfo 结构体切片来存储结果
	appointmentsInfo := []AppointmentInfo{}

	// 根据每一个项目的 ID 查询 Redis 中的数据
	for _, item := range sftis {
		// 构建 Redis 中存储项目预约信息的键名
		rKey := fmt.Sprintf(types.RedisItem, strconv.FormatInt(item.ID, 10))
		// 查询 Redis 中存储的预约信息
		// 使用 LRANGE 命令获取列表中的所有元素
		appointments, err := myredis.Client.LRange(c, rKey, 0, -1).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		// 检查每个预约信息是否匹配当前用户的 ID，并记录排队位置
		flag := 1
		for i, appointment := range appointments {
			// 如果预约信息中包含了当前用户的 ID，则记录排队位置，并将项目名和排队位置添加到结果中
			if appointment == strconv.FormatInt(user.ID, 10) {
				info := AppointmentInfo{
					ItemName:   item.Item,
					ItemTime:   item.AvgTimePerPerson,
					ItemMax:    item.MaxParticipants,
					QueueIndex: i + 1, // 索引从 0 开始，排队位置从 1 开始计数
					ItemId:     strconv.FormatInt(item.ID, 10),
				}
				appointmentsInfo = append(appointmentsInfo, info)
				flag = 0
				break // 找到匹配项后可以跳出内部循环，因为每个用户只能预约一次
			}
		}
		if flag == 1 {
			info := AppointmentInfo{
				ItemName:   item.Item,
				ItemTime:   item.AvgTimePerPerson,
				ItemMax:    item.MaxParticipants,
				QueueIndex: 0, // 未预约，排队位置为 0
			}
			appointmentsInfo = append(appointmentsInfo, info)
		}
	}
	// 返回预约信息列表
	return appointmentsInfo, nil
}

func HandleQueue(c *gin.Context) {
	var (
		err error
		req ItemReq
	)
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}
	rKey := fmt.Sprintf(types.RedisItem, req.Id)
	//循环删除
	for i := 0; i < len(req.Ids); i++ {
		flag := myredis.Client.LRem(c, rKey, 0, req.Ids[i])
		if flag.Val() == 0 {
			response.RespErrorWithMsg(c, code.ServerErrCache, "未找到该用户，请刷新")
			return
		}
	}
	response.RespSuccess(c, "删除成功")
}
