package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/invite"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/rechargecampaignrule"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/transferorder"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"net/http"
	"strconv"
	"time"
	"vpay/configs"
	"vpay/internal/db"
	"vpay/internal/handlers"
	"vpay/internal/logic"
	"vpay/internal/myredis"
	"vpay/internal/types"
	"vpay/utils"
	"vpay/utils/db_utils"
)

const (
	UnPending     code.MyCode = 30021
	GetUserIdFail code.MyCode = 30025
)

func GetTransferOrder(c *gin.Context) {
	userID, err := getUserId(c)
	if err != nil {
		return
	}
	var req types.GetTransferOrderReq
	if err = c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	transferOrderID, _ := strconv.ParseInt(req.OrderId, 10, 64)
	logrus.Info(transferOrderID)
	transferOrder, err := db.DB.TransferOrder.Query().Where(transferorder.DeletedAt(utils.ZeroTime), transferorder.ID(transferOrderID)).First(c)
	if err != nil {
		logrus.Error(err)
		response.RespErrorWithMsg(c, code.ServerErrDB, "数据库查询异常")
		return
	}
	if transferOrder.TargetUserID != userID && transferOrder.SourceUserID != userID {
		response.RespErrorWithMsg(c, code.AuthFailed, "非本人关联订单，无法查询")
		return
	}
	respData := serializer.FrontendSerialize(transferOrder)
	response.RespSuccess(c, respData)
}

func CreateTransferOrder(c *gin.Context) {
	userID, err := getUserId(c)
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}

	var req types.CreateTransferOrderReq
	if err = c.ShouldBindJSON(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 添加充值金额限制 fixme: 临时写到常量，正常需要写到数据库中配置
	if req.Amount < types.RechargeLimitLowest {
		response.RespErrorInvalidParams(c, errors.New("充值金额小于最低限制金额"))
		return
	}

	symbolID := types.CepSymbolID
	if req.SymbolID != nil {
		symbolID, err = strconv.ParseInt(*req.SymbolID, 10, 64)
		if err != nil {
			response.DynamicRespErr(c, err)
			return
		}
	}

	var transferOrder *cep_ent.TransferOrder
	// 如果是手动充值的，就直接成功，从 genesis 账户转账
	switch req.Type {
	case enums.TransferOrderTypeManual:
		var targetUserID int64
		if req.TargetUserID != nil {
			targetUserID, err = strconv.ParseInt(*req.TargetUserID, 10, 64)
			if err != nil {
				response.DynamicRespErr(c, err)
				return
			}
		}

		transferOrder, err = handlers.CreateManualTransferOrder(c, &handlers.CreateManualTransferOrderOptions{
			Tx:           nil,
			SymbolID:     symbolID,
			Amount:       req.Amount,
			TargetUserID: targetUserID,
			SourceUserID: types.GenesisUserID,
		})
		if err != nil {
			response.DynamicRespErr(c, err)
			return
		}
	case enums.TransferOrderTypeRecharge:
		// 充值订单只能给自己充值
		transferOrder, err = handlers.CreatePendingTransferOrder(c, &handlers.CreatePendingTransferOrderOptions{
			Tx:           nil,
			SymbolID:     symbolID,
			Amount:       req.Amount,
			TargetUserID: userID,
			Type:         enums.TransferOrderTypeRecharge,
		})
		if err != nil {
			response.DynamicRespErr(c, err)
			return
		}
		// 待充值的订单如果在一定时间后还是待定状态 pending，就需要被取消掉
		go logic.CancelPendingUnknownTransferOrderInTime(configs.Ctx, transferOrder.ID)
	default:
		var targetUserID int64
		if req.TargetUserID != nil {
			targetUserID, err = strconv.ParseInt(*req.TargetUserID, 10, 64)
			if err != nil {
				response.DynamicRespErr(c, err)
				return
			}
		} else {
			response.RespErrorInvalidParams(c, errors.New("unknown transfer_order"))
		}
		// 如果都不是，就先创建一个 unknown 类型的订单，这个订单状态是 pending，它还会有后续的动作
		transferOrder, err = handlers.CreatePendingTransferOrder(c, &handlers.CreatePendingTransferOrderOptions{
			Tx:           nil,
			SymbolID:     symbolID,
			Amount:       req.Amount,
			TargetUserID: targetUserID,
			Type:         enums.TransferOrderTypeUnknown,
		})
		if err != nil {
			response.DynamicRespErr(c, err)
			return
		}
		// 未知的订单如果在一定时间后还是待定状态 pending，就需要被取消掉
		go logic.CancelPendingUnknownTransferOrderInTime(configs.Ctx, transferOrder.ID)
	}
	if req.IsActive == true {
		if err = myredis.Client.Set(c, fmt.Sprintf(types.RedisTransferIsActive, transferOrder.SerialNumber), true, 10*time.Minute).Err(); err != nil {
			logrus.Error(err)
		}
	}
	response.RespSuccess(c, serializer.FrontendSerialize(transferOrder))
}

// PostThirdPartyOrderOfTransferOrder 调起第三方订单，关联到本地的订单
func PostThirdPartyOrderOfTransferOrder(c *gin.Context) {
	// 第三方订单都是充值类型的订单，所以要检查调用的用户是订单的目标用户
	userID, err := getUserId(c)
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	var req struct {
		ID   int64                   `uri:"id"`
		Code string                  `json:"code"`
		Type enums.TransferOrderType `json:"type"`
	}
	if err = c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	var apiReturn *types.ApiReturn
	var transferOrder *cep_ent.TransferOrder
	err = db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		transferOrder, err = tx.TransferOrder.Query().Where(transferorder.DeletedAt(utils.ZeroTime), transferorder.ID(req.ID)).First(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "数据库查询异常")
			logrus.Error(err)
			return err
		}
		if transferOrder.TargetUserID != userID {
			response.RespErrorWithMsg(c, code.AuthFailed, "非本人订单，无法查询")
			logrus.Error(err)
			return err
		}
		startTime := transferOrder.CreatedAt.Add(8 * time.Hour).Format("20060102150405")
		expireTime := transferOrder.CreatedAt.Add(8*time.Hour + 5*time.Minute).Format("20060102150405")
		logrus.Infof("createAt is: %+v", transferOrder.CreatedAt)
		logrus.Infof("start time : %v ,expire time: %v", startTime, expireTime)
		switch req.Type {
		case enums.TransferOrderTypeRechargeVX:
			openID, err := handlers.GetOpenIdByJsApi(req.Code)
			if err != nil {
				response.RespErrorWithMsg(c, code.ServerErr, "服务失败")
				logrus.Error(err)
				return err
			}
			apiReturn, err = handlers.MakeVxPlatformPayOrder(&types.MakeVxPlatformPayOrderOptions{
				OpenID:      openID,
				AppID:       configs.Conf.VxAppConfig.CephalonVxPlatform.AppID,
				TotalAmount: transferOrder.Amount / 10,
				Describe:    "微信公众平台支付",
				OutTradeNo:  transferOrder.SerialNumber,
				TimeStart:   startTime,
				TimeExpire:  expireTime,
				CallBackUrl: "/v1/orders/transfers/call-back",
			})
		case enums.TransferOrderTypeRechargeAlipay:
			alipayUserID, err := handlers.AlipayGetUserID(req.Code)
			if err != nil {
				logrus.Infof("occure an error: %v", err)
				response.RespErrorWithMsg(c, GetUserIdFail, "获取用户 ID 失败")
				logrus.Error(err)
				return err
			}
			apiReturn, err = handlers.MakeAlipayPayOrder(&types.MakeAliPayOrderOptions{
				TotalAmount: transferOrder.Amount / 10,
				Describe:    "支付宝支付",
				OutTradeNo:  transferOrder.SerialNumber,
				TimeStart:   startTime,
				TimeExpire:  expireTime,
				BuyerId:     alipayUserID,
				Context:     c,
				CallBackUrl: "/v1/orders/transfers/call-back",
			})
		default:
			response.RespErrorWithMsg(c, code.InvalidParams, "not supported recharge type")
			return nil
		}

		err = tx.TransferOrder.Update().Where(transferorder.SerialNumber(transferOrder.SerialNumber), transferorder.DeletedAt(utils.ZeroTime)).SetType(req.Type).Exec(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErr, "发起第三方支付失败")
			logrus.Error(err)
			return err
		}
		thirdApiReturn, err := json.Marshal(apiReturn)
		if err != nil {
			DynamicResponseErr(c, err)
			logrus.Error(err)
			return err
		}
		transferOrder, err = tx.TransferOrder.UpdateOne(transferOrder).SetThirdAPIResp(string(thirdApiReturn)).Save(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, "数据库更新失败")
			logrus.Error(err)
			return err
		}
		apiReturn.SerialNumber = transferOrder.SerialNumber
		response.RespSuccess(c, serializer.FrontendSerialize(apiReturn))
		return nil
	})
	if err != nil {
		return
	}

}

// CloseOrDeleteTransferOrder 关闭后台普通充值订单
func CloseOrDeleteTransferOrder(c *gin.Context) {
	userID, err := getUserId(c)
	if err != nil {
		return
	}
	var req types.CloseRechargeOrderReq
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err = db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		transferOrderID, err := strconv.ParseInt(req.OrderId, 10, 64)
		transferOrder, err := tx.TransferOrder.Query().Where(transferorder.DeletedAt(utils.ZeroTime), transferorder.ID(transferOrderID)).First(c)
		if err != nil {
			logrus.Error(err)
			response.RespErrorWithMsg(c, code.ServerErrDB, "数据库查询异常")
			return err
		}
		if transferOrder.TargetUserID != userID {
			logrus.Error(err)
			response.RespErrorWithMsg(c, code.AuthFailed, "非本人订单，无法操作")
			return err
		}
		if transferOrder.Status != transferorder.StatusPending {
			logrus.Infof("该订单已脱离初始状态，关闭失败")
			response.RespErrorWithMsg(c, UnPending, "该订单已脱离初始状态，关闭失败")
			return err
		}
		transferOrder, err = tx.TransferOrder.UpdateOne(transferOrder).SetStatus(transferorder.StatusCanceled).Save(c)
		if err != nil {
			logrus.Error(err)
			response.RespErrorWithMsg(c, code.ServerErrDB, "关闭订单失败")
			return err
		}
		if req.IsDelete != nil && *req.IsDelete == true {
			transferOrder, err = tx.TransferOrder.UpdateOne(transferOrder).SetDeletedAt(time.Now()).Save(c)
			if err != nil {
				logrus.Error(err)
				response.RespErrorWithMsg(c, code.ServerErrDB, "删除订单失败")
				return err
			}
		}
		logrus.Infof("关闭成功")
		response.RespSuccess(c, serializer.FrontendSerialize(transferOrder))
		return nil
	}); err != nil {
		logrus.Error(err)
	}
}

func TransferOrderReceiveCallBack(c *gin.Context) {
	var req types.ReceiveCallBackReq
	err := c.ShouldBind(&req)
	if err != nil {
		logrus.Infof("occure err : %v", err)
		c.JSON(http.StatusOK, "fail")
		return
	}
	logrus.Infof("req: %+v", req)
	if req.Status != "0" {
		logrus.Infof("pay result is fail: status != 0: message: %v", req.Message)
		c.JSON(http.StatusOK, "success")
		return
	}
	if req.ResultCode != "0" {
		logrus.Infof("pay result is fail: resultcode != 0: err_code: %v, err_msg: %v", req.ErrCode, req.ErrMsg)
		c.JSON(http.StatusOK, "success")
		return
	}
	if req.PayResult != 0 {
		logrus.Infof("pay result is fail: result != 0")
		c.JSON(http.StatusOK, "success")
		return
	}
	if txErr := db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {

		transferOrder, err := tx.TransferOrder.Query().Where(transferorder.SerialNumber(req.OutTradeNo)).First(c)
		if err != nil {
			logrus.Error(err)
			return err
		}
		if transferOrder.Status == transferorder.StatusSucceed {
			return nil
		}

		if err = tx.TransferOrder.UpdateOne(transferOrder).
			SetDeletedAt(utils.ZeroTime).
			SetStatus(transferorder.StatusSucceed).
			SetOutTransactionID(req.OutTransactionId).
			Exec(c); err != nil {
			return err
		}

		var billWay enums.BillWay
		logrus.Infof("recharge order type is : %v", transferOrder.Type)
		switch transferOrder.Type {
		case enums.TransferOrderTypeRechargeVX:
			billWay = enums.BillWayRechargeWechat
		case enums.TransferOrderTypeRechargeAlipay:
			billWay = enums.BillWayRechargeAlipay
		default:
			billWay = enums.BillWayUnknown
		}

		// 由于订单成功了，创建对应的流水并更新币种钱包余额
		_, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
			Tx:           tx,
			TargetUserID: transferOrder.TargetUserID,
			SourceUserID: types.GenesisUserID,
			SymbolID:     transferOrder.SymbolID,
			Amount:       transferOrder.Amount,
			Type:         enums.BillTypeRecharge,
			Way:          billWay,
			InviteID:     0,
			OrderID:      transferOrder.ID,
		})
		if err != nil {
			return err
		}

		// 如果在活动期间，发放抽奖次数奖励
		err = handlers.CreateOrAddLottoCount(c, tx, &types.CreateOrAddLottoCountOption{
			UserID:         transferOrder.TargetUserID,
			Type:           enums.LottoConditionRecharge,
			RechargeAmount: transferOrder.Amount / 1000,
		})
		if err != nil {
			logrus.Errorf("failed to add lotto count, err: %v", err)
			return err
		}

		// 查询充值用户
		_user, err := tx.User.Query().Where(user.DeletedAt(utils.ZeroTime), user.ID(transferOrder.TargetUserID)).First(c)
		if err != nil {
			return err
		}

		// 根据用户渠道来进行赠送
		{
			if _user.ParentID == 1694591015162748928 && transferOrder.Amount/1000 >= 100 {
				_, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
					Tx:           tx,
					TargetUserID: transferOrder.TargetUserID,
					SourceUserID: types.GenesisUserID,
					SymbolID:     transferOrder.SymbolID,
					Amount:       transferOrder.Amount * 5 / 100,
					Type:         enums.BillTypeActive,
					Way:          enums.BillWayActiveRecharge,
					InviteID:     0,
					OrderID:      transferOrder.ID,
				})
				if err != nil {
					return err
				}
			}
		}
		// 如果用户是第一次充值，还要再送其邀请人的一些 cep 作为营销奖励
		// 用户是首充
		if _user.IsRecharge == false {
			parentUser, err := tx.User.Query().Where(user.DeletedAt(utils.ZeroTime), user.ID(_user.ParentID)).First(c)
			if err != nil && !cep_ent.IsNotFound(err) {
				return err
			}
			// 该用户有邀请者并且还有活动时，送奖励
			if parentUser != nil {

				var firstGiftCep int64
				// 查询该用户邀请注册活动的邀请码信息
				campaignType := "share_register" // fixme: 活动类型需要跟着活动走
				inviteInfo, err := tx.Invite.Query().Where(invite.DeletedAt(utils.ZeroTime), invite.UserID(parentUser.ID), invite.Type(campaignType)).First(c)
				if cep_ent.IsNotFound(err) {
					firstGiftCep = 0
				} else if err != nil {
					logrus.Errorf("query invite failed, err: %v", err)
					return err
				} else {
					firstGiftCep = inviteInfo.FirstRechargeCep
				}
				_, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
					Tx:           tx,
					TargetUserID: parentUser.ID,
					SourceUserID: types.GenesisUserID,
					SymbolID:     transferOrder.SymbolID,
					Amount:       firstGiftCep,
					Type:         enums.BillTypeActive,
					Way:          enums.BillWayFirstInviteRecharge,
					InviteID:     inviteInfo.ID,
					OrderID:      transferOrder.ID,
				})
				if err != nil {
					return err
				}

			}

			// 记录用户已充值过
			if err = tx.User.UpdateOne(_user).SetIsRecharge(true).Exec(c); err != nil {
				return err
			}
		}

		location, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			logrus.Error(err)
			return err
		}
		fakeNow := time.Date(2024, 2, 10, 0, 0, 0, 0, location)
		activityBeginAt := time.Date(2024, 2, 9, 0, 0, 0, 0, location)
		activityEndedAt := time.Date(2024, 2, 25, 0, 0, 0, 0, location)

		// 活动充值
		if fakeNow.After(activityBeginAt) && fakeNow.Before(activityEndedAt) {
			_, err = myredis.Client.Get(c, fmt.Sprintf(types.RedisTransferIsActive, transferOrder.SerialNumber)).Result()
			if errors.Is(err, redis.Nil) {
				logrus.Info("redis 查询结果是空")
			} else if err != nil {
				logrus.Error(err)
				return err
			} else if transferOrder.Amount/1000 >= 100 && transferOrder.Amount/1000 < 1000 {
				_, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
					Tx:           tx,
					TargetUserID: transferOrder.TargetUserID,
					SourceUserID: types.GenesisUserID,
					SymbolID:     transferOrder.SymbolID,
					Amount:       transferOrder.Amount,
					Type:         enums.BillTypeActive,
					Way:          enums.BillWayActiveRecharge,
					InviteID:     0,
					OrderID:      transferOrder.ID,
				})
				if err != nil {
					return err
				}
				// 活动赠送成功直接返回
				return nil
			} else if transferOrder.Amount/1000 >= 1000 {
				_, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
					Tx:           tx,
					TargetUserID: transferOrder.TargetUserID,
					SourceUserID: types.GenesisUserID,
					SymbolID:     transferOrder.SymbolID,
					Amount:       transferOrder.Amount * 3 / 2,
					Type:         enums.BillTypeActive,
					Way:          enums.BillWayActiveRecharge,
					InviteID:     0,
					OrderID:      transferOrder.ID,
				})
				if err != nil {
					return err
				}
				// 活动赠送成功直接返回
				return nil
			}
		}
		var giftCep int64
		// 根据充值金额计算出赠送的 cep
		// 查询赠送比例（活动规则中单位是元，请求是参数单位是 cep，所以查询条件需要换算，除以 1000）
		rechargeCampaignRule, err := tx.RechargeCampaignRule.Query().Where(rechargecampaignrule.DeletedAt(utils.ZeroTime), rechargecampaignrule.LittleValueLTE(transferOrder.Amount/1000), rechargecampaignrule.LargeValueGT(transferOrder.Amount/1000)).First(c)
		if cep_ent.IsNotFound(err) {
			// 此笔充值金额没有营销规则内
			giftCep = 0
		} else if err != nil {
			logrus.Errorf("query recharge_campaign_rule failed, err: %v", err)
			return err
		} else {
			// 计算赠送的 cep
			giftCep = transferOrder.Amount * rechargeCampaignRule.GiftPercent / 100
		}
		logrus.Infof("gift cep is : %v", giftCep)
		// 有活动、营销账户还有钱、赠送的 cep 大于 0 才会有赠送
		if giftCep > 0 {

			_, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
				Tx:           tx,
				TargetUserID: transferOrder.TargetUserID,
				SourceUserID: types.GenesisUserID,
				SymbolID:     transferOrder.SymbolID,
				Amount:       giftCep,
				Type:         enums.BillTypeActive,
				Way:          enums.BillWayActiveRecharge,
				InviteID:     0,
				OrderID:      transferOrder.ID,
			})
			if err != nil {
				return err
			}
		}

		return nil
	}); txErr != nil {

	}
	c.JSON(http.StatusOK, "success")
}
