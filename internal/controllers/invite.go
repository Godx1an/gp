package controllers

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/invite"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/transferorder"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/response"
	"slices"
	"vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/handlers"
	"vpay/internal/logic"
	"vpay/internal/types"
	"vpay/utils"
	"vpay/utils/db_utils"
)

// ListInviteCode 邀请码列表
func ListInviteCode(c *gin.Context) {
	var req types.ListInviteCodeReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	query := db.DB.Invite.Query().Where(invite.DeletedAt(utils.ZeroTime))
	if req.InviteCode != "" {
		query = query.Where(invite.InviteCodeEQ(req.InviteCode))
	}
	if req.Type != "" {
		query = query.Where(invite.TypeEQ(req.Type))
	}

	total, err := query.Count(c)
	if err != nil {
		logrus.Errorf("query invite code count failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	invites, err := query.Offset((req.PageIndex - 1) * req.PageSize).Limit(req.PageSize).All(c)
	if err != nil {
		logrus.Errorf("query invite code failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	var resp common_types.PaginateResp
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = total
	resp.List = invites

	response.RespSuccess(c, resp)
}

func GetUserInfoByInvite(c *gin.Context) {
	var req types.GetUserInfoByInviteCodeReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	inviteInfo, err := db.DB.Invite.Query().Where(invite.InviteCodeEQ(req.InviteCode)).First(c)
	if cep_ent.IsNotFound(err) {
		response.RespErrorWithMsg(c, code.NotFound, "邀请码信息不存在")
		return
	} else if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	userInfo, err := db.DB.User.Query().Where(user.ID(inviteInfo.UserID)).First(c)
	if cep_ent.IsNotFound(err) {
		response.RespErrorWithMsg(c, code.NotFound, "邀请人信息不存在")
		return
	} else if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, userInfo)
}

// GetBindInfo 获取绑定信息
func GetBindInfo(ctx *gin.Context) {
	userId, err := common.GetUserId(ctx)
	if err != nil {
		response.RespError(ctx, code.UnLogin)
		return
	}

	_user, err := db.DB.User.Query().Where(user.IDEQ(userId)).First(ctx)
	if err != nil {
		logrus.Errorf("failed to query user, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	var resp types.BindInviteCodeResp
	if _user.ParentID == 0 {
		resp.ID = _user.ID
		resp.NickName = _user.NickName
		resp.Amount = 0
		resp.ParentPhone = ""
		response.RespSuccessWithMsg(ctx, resp, "user did not invited by anyone")
		return
	}

	// 查询当前用户邀请人信息
	parent, err := db.DB.User.Query().Where(user.IDEQ(_user.ParentID)).First(ctx)
	if err != nil {
		logrus.Errorf("failed to query user parent, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	resp.ID = _user.ID
	resp.Amount = 0
	resp.ParentPhone = utils.MaskPhone(parent.Phone)
	response.RespSuccess(ctx, resp)
}

// BindInviteCode 绑定邀请码
func BindInviteCode(ctx *gin.Context) {
	userId, err := common.GetUserId(ctx)
	if err != nil {
		response.RespError(ctx, code.UnLogin)
		return
	}

	var req types.BindInviteCodeReq
	if err = ctx.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}

	txErr := db_utils.WithTx(ctx, nil, func(tx *cep_ent.Tx) error {
		// 判断邀请码是否存在
		_invite, err := tx.Invite.Query().Where(invite.InviteCodeEQ(req.InviteCode)).First(ctx)
		if cep_ent.IsNotFound(err) {
			logrus.Warningf("the user enters a non-existent invitation code, err: %v", err)
			response.RespErrorWithMsg(ctx, code.NotFound, errors.New("invite code not found").Error())
			return nil
		}
		if err != nil {
			logrus.Errorf("failed to query table invite, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}

		var resp types.BindInviteCodeResp

		// 判断当前用户是否有邀请人
		currentUser, err := tx.User.Query().Where(user.IDEQ(userId)).First(ctx)
		if err != nil {
			logrus.Errorf("failed to query user, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}
		if currentUser.ParentID != 0 {
			// 已经绑定邀请关系了
			response.RespErrorWithMsg(ctx, code.SourceExist, "用户已绑定邀请关系")
			return nil
		}

		// 查询当前用户邀请人信息
		bindUser, err := tx.User.Query().Where(user.IDEQ(_invite.UserID)).First(ctx)
		if cep_ent.IsNotFound(err) {
			response.RespErrorWithMsg(ctx, code.NotFound, "邀请码不存在")
			return err
		} else if err != nil {
			logrus.Errorf("failed to query user parent, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}

		if bindUser.ID == currentUser.ID {
			resp.ID = currentUser.ID
			resp.Amount = 0
			resp.ParentPhone = ""
			response.RespErrorWithMsg(ctx, code.ServerErr, "users cannot invite themselves")
			return nil
		}
		// 递归检查邀请者的上级不能有自己，也就是邀请人不能是自己的下级之一，注意该递归不查第一个 parent 就是自己的情况
		parentNotLoop, err := checkParentIDIsNotMyChild(ctx, tx.Client(), currentUser.ID, bindUser)
		if err != nil {
			response.DynamicRespErr(ctx, err)
			return nil
		}
		if !parentNotLoop {
			response.RespErrorWithMsg(ctx, code.ServerErr, "users cannot invite by loop")
			return nil
		}
		// 更新当前用户的上级
		{
			shareCep := _invite.ShareCep
			// 更新邀请人的 wallet 并产生流水
			_, err = logic.GenBillAndUpdateWallet(ctx, &logic.GenBillAndUpdateWalletOptions{
				Tx:           tx,
				TargetUserID: bindUser.ID,
				SourceUserID: types.GenesisUserID,
				SymbolID:     types.CepSymbolID,
				Amount:       shareCep,
				Type:         enums.BillTypeActive,
				Way:          enums.BillWayActiveShare,
				InviteID:     _invite.ID,
				OrderID:      0,
			})
			if err != nil {
				return code.ServerErrDB
			}

			// 如果在活动期间，发放抽奖次数奖励
			err = handlers.CreateOrAddLottoCount(ctx, tx, &types.CreateOrAddLottoCountOption{
				UserID: bindUser.ID,
				Type:   enums.LottoConditionInviteRegister,
			})

		}
		// 更新当前用户邀请人
		_user, err := tx.User.UpdateOneID(userId).SetParentID(_invite.UserID).Save(ctx)
		if err != nil {
			logrus.Errorf("failed to update user parent id, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}

		// 发放奖励
		_bill, err := logic.GenBillAndUpdateWallet(ctx, &logic.GenBillAndUpdateWalletOptions{
			Tx:           tx,
			TargetUserID: userId,
			SourceUserID: types.GenesisUserID,
			SymbolID:     types.CepSymbolID,
			Amount:       _invite.RegCep,
			Type:         enums.BillTypeActive,
			Way:          enums.BillWayActiveBind,
			InviteID:     _invite.ID,
			OrderID:      0,
		})
		if err != nil {
			return err
		}

		resp.ID = _user.ID
		resp.NickName = _user.NickName
		resp.Amount = _bill.Amount
		resp.ParentPhone = utils.MaskPhone(bindUser.Phone)

		response.RespSuccess(ctx, resp)

		return nil
	})
	if txErr != nil {
		return
	}

}

func checkParentIDIsNotMyChild(ctx context.Context, dbClient *cep_ent.Client, currentUserID int64, parent *cep_ent.User) (bool, error) {
	if parent.ParentID != 0 {
		if parent.ParentID == currentUserID {
			return false, nil
		} else {
			newParent, err := dbClient.User.Query().Where(user.IDEQ(parent.ParentID)).First(ctx)
			if err != nil {
				return false, err
			}
			return checkParentIDIsNotMyChild(ctx, dbClient, currentUserID, newParent)
		}
	} else {
		// 已经没有上级了，就证明没问题
		return true, nil
	}
}

func GetChannelBonus(ctx *gin.Context) {
	userId, err := common.GetUserId(ctx)
	if err != nil {
		response.RespError(ctx, code.UnLogin)
		return
	}
	_user, err := db.DB.User.Query().Where(user.IDEQ(userId)).First(ctx)
	if err != nil {
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	var channels = []string{"13560234562", "18270873187", "18183021520", "18111291337", "13911618018", "17758892234"}
	isChannel := false
	if slices.Contains(channels, _user.Phone) {
		isChannel = true
	}
	if !isChannel {
		resp := struct {
			IsChannel bool `json:"is_channel"`
		}{isChannel}
		response.RespSuccess(ctx, resp)
		return
	}
	var count int64
	users, err := db.DB.User.Query().Where(user.ParentID(userId)).All(ctx)
	for _, _user := range users {
		transferOrders, err := db.DB.TransferOrder.Query().Where(transferorder.TargetUserID(_user.ID), transferorder.StatusNEQ(transferorder.StatusSucceed)).All(ctx)
		if cep_ent.IsNotFound(err) {
			continue
		} else if err != nil {
			response.RespError(ctx, code.ServerErrDB)
			return
		} else if len(transferOrders) != 0 && transferOrders != nil {
			for _, transferOrder := range transferOrders {
				count = transferOrder.Amount + count
			}
		}

	}
	resp := struct {
		Count int64 `json:"count"`
	}{count}
	response.RespSuccess(ctx, resp)
}

func IsChannel(ctx *gin.Context) {
	userId, err := common.GetUserId(ctx)
	if err != nil {
		response.RespError(ctx, code.UnLogin)
		return
	}
	_user, err := db.DB.User.Query().Where(user.IDEQ(userId)).First(ctx)
	if err != nil {
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	var channels = []string{"13560234562", "18270873187", "18183021520", "18111291337", "13911618018", "17758892234"}
	isChannel := false
	if slices.Contains(channels, _user.Phone) {
		isChannel = true
	}
	resp := struct {
		IsChannel bool `json:"is_channel"`
	}{isChannel}
	response.RespSuccess(ctx, resp)
}
