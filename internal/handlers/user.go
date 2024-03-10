package handlers

import (
	"context"
	"errors"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"github.com/Godx1an/gp_ent/pkg/ent_work/user"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"graduation_project/configs"
	"graduation_project/internal/myredis"
	"graduation_project/utils"
	"graduation_project/utils/db_utils"
	"strconv"
)

type RegisterOptions struct {
	Tx             *ent_work.Tx
	Phone          string
	Password       string
	IsEnterprise   bool
	EnterpriseName string
	Ticket         string
	//MasterIdStr    string
	InviteCode string
	Email      string
	AreaCode   string
}

func Register(ctx context.Context, tx *ent_work.Tx, opt *RegisterOptions) (*ent_work.User, error) {
	var (
		_user *ent_work.User
		err   error
	)
	if err = db_utils.WithTx(ctx, tx, func(tx *ent_work.Tx) error {
		var (
			parentId string
		)
		// 检查 opt 是否携带电话号码
		{
			if opt.Phone == "" {
				return errors.New("注册操作请填写手机号")
			}
		}
		// 检查电话号是否被注册
		{
			_user, err = tx.User.Query().Where(user.Phone(opt.Phone)).First(ctx)
			if err != nil && !ent_work.IsNotFound(err) {
				logrus.Error(err)
				return code.ServerErrDB
			}
			if _user != nil {
				logrus.Error(err)
				return code.FailHasRegister
			}
		}

		userId := utils.GenSnowflakeID()

		if err != nil {
			return code.ServerErr
		}
		create := tx.User.Create().
			SetID(userId).
			SetPhone(opt.Phone).
			SetPassword(opt.Password)
		// 新增邮箱用户
		if opt.Email != "" && opt.AreaCode != "" {
			// 检查邮箱是否被注册
			{
				_user, err = tx.User.Query().Where(user.Email(opt.Email)).First(ctx)
				if err != nil && !ent_work.IsNotFound(err) {
					logrus.Error(err)
					return code.ServerErrDB
				}
				if _user != nil {
					logrus.Error(err)
					return code.FailHasRegister
				}
			}
			create.SetEmail(opt.Email).SetAreaCode(opt.AreaCode)
		}
		// 新增用户
		_user, err = create.Save(ctx)
		if err != nil {
			logrus.Error(err)
			return code.ServerErrDB
		}

		if opt.Ticket != "" {
			openid, err := myredis.Client.Get(ctx, opt.Ticket).Result()
			if err == redis.Nil {
				return nil
			} else if err != nil {
				return code.ServerErrCache
			}
			_, err = tx.VXSocial.Create().SetAppID(configs.Conf.VxAppConfig.CephalonVxPlatform.AppID).SetOpenID(openid).SetUserID(_user.ID).Save(ctx)
			if err != nil {
				return code.ServerErrDB
			}
		}
		if parentId != "" {
			masterId, err := strconv.ParseInt(parentId, 10, 64)
			if err != nil {
				return code.ServerErr
			}
			exist, err := tx.User.Query().Where(user.ID(masterId)).Exist(ctx)
			if err != nil {
				return code.ServerErrDB
			}
			if !exist {
				logrus.Info("邀请方不存在")
				return nil
			}
			_user, err = tx.User.UpdateOne(_user).SetParentID(masterId).Save(ctx)
			if err != nil {
				return code.ServerErrDB
			}
		}
		return nil
	}); err != nil {
		logrus.Errorf("db err: %v", err)
		return nil, err
	}
	logrus.Info("注册成功")
	return _user, nil
}
