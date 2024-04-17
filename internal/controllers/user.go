package controllers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/school"
	"github.com/Godx1an/gp_ent/pkg/ent_work/schoolfitnesstestitem"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"graduation_project/internal/handlers"
	"graduation_project/internal/myredis"
	"graduation_project/internal/types"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
		_, err = tx.User.UpdateOne(_user).SetSchool(_school.Name).Save(c)
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

type QueryResponse struct {
	Sftis []*ent_work.SchoolFitnessTestItem `json:"sftis"`
	Lists []int64                           `json:"lists"`
}

func QueryItem(c *gin.Context) {

	var (
		acc     account
		_user   *ent_work.User
		_school *ent_work.School
		err     error
		sftis   []*ent_work.SchoolFitnessTestItem
		lists   []int64
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
		sftis, err = tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.DeletedAt(utils.ZeroTime)).Where(schoolfitnesstestitem.SchoolID(_school.ID)).All(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		lists, err = GetListsCount(c, sftis)
		if err != nil {
			return err
		}
		responseData := QueryResponse{
			Sftis: sftis,
			Lists: lists,
		}
		response.RespSuccessWithMsg(c, responseData, "查询成功")
		return nil
	}); err != nil {
		return
	}
	return
}

func QueryItemTime(c *gin.Context) {
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
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		sftis, err := tx.SchoolFitnessTestItem.Query().Where(schoolfitnesstestitem.DeletedAt(utils.ZeroTime)).Where(schoolfitnesstestitem.School(_user.School)).All(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		lists, err := GetListsCount(c, sftis)
		if err != nil {
			return err
		}
		response.RespSuccess(c, lists)
		return nil
	}); err != nil {
		return
	}
}

func GetListsCount(c *gin.Context, sftis []*ent_work.SchoolFitnessTestItem) ([]int64, error) {
	var listsCount []int64

	for _, item := range sftis {
		// 构建 Redis Key
		rKey := fmt.Sprintf(types.RedisItem, strconv.FormatInt(item.ID, 10))

		// 使用 LLEN 命令获取列表的长度
		count, err := myredis.Client.LLen(c, rKey).Result()
		if err != nil {
			logrus.Errorf("failed to get list length for key (%s), err: %v", rKey, err)
			return nil, errors.New("failed to get list length from Redis")
		}
		// 将列表的长度添加到结果中
		listsCount = append(listsCount, count)
	}

	return listsCount, nil
}

func UploadAvatar(c *gin.Context) {
	var (
		err   error
		_user *ent_work.User
	)
	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}
	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		return nil
	}); err != nil {
		return
	}
	err = c.Request.ParseMultipartForm(10 << 20)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "文件过大，解析错误")
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "无法获取文件")
		return
	}
	// 将路径中的 '\' 替换为 '/'
	savePath := "D:/gp_front/vue-base/src/assets/avatar" + strconv.FormatInt(_user.ID, 10) + ".jpg"
	newFile, err := os.Create(savePath)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "无法创建文件")
		return
	}
	defer newFile.Close()
	_, err = io.Copy(newFile, file)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "文件写入失败")
		return
	}
	response.RespSuccessWithMsg(c, nil, "上传成功")
	return
}

func FindImage(c *gin.Context) {
	var (
		err   error
		_user *ent_work.User
	)
	userID, _ := c.Get("user_id")
	UID, ok := userID.(int64)
	if !ok {
		response.RespErrorInvalidParams(c, code.ServerErr)
		return
	}
	if err = db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		_user, err = tx.User.Query().Where(user.ID(UID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "无法查询到数据")
			return errors.New("无法查询到数据")
		}
		return nil
	}); err != nil {
		return
	}

	// 获取文件路径
	// 构建要查找的文件名
	fileName := "avatar" + strconv.FormatInt(_user.ID, 10) + ".jpg"
	// 指定要搜索的目录
	dir := "D:/gp_front/vue-base/src/assets"
	// 默认图片路径
	defaultImagePath := "D:/gp_front/vue-base/src/assets/defaultAvatar.jpg"
	// 最终图片路径
	imagePath := defaultImagePath

	// 遍历目录中的文件
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 如果是文件并且文件名匹配，则设置图片路径为该文件路径
		if !info.IsDir() && info.Name() == fileName {
			imagePath = path
		}
		return nil
	})
	if err != nil {
		return
	}
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "无法读取图片文件")
		return
	}
	base64Str := base64.StdEncoding.EncodeToString(imageData)
	response.RespSuccessWithMsg(c, base64Str, "查询成功")
	// 返回图片路径
	return
}

func Dequeue(c *gin.Context) {
	var (
		req ItemReq
		err error
	)
	err = c.ShouldBind(&req)
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
	rKey := fmt.Sprintf(types.RedisItem, req.Id)
	fmt.Println(rKey)
	flag := myredis.Client.LRem(c, rKey, 0, UID)
	if flag.Val() == 0 {
		response.RespErrorWithMsg(c, code.ServerErrCache, "取消失败，请刷新")
		return
	}
	response.RespSuccessWithMsg(c, nil, "取消成功")
	return
}
