package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/mission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/renewalagreement"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/wallet"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/response"
	"strconv"
	"time"
	"vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/types"
	"vpay/utils"
)

// ListRenewalAgreementMission 获取用户任务协议列表
func ListRenewalAgreementMission(c *gin.Context) {
	var req common_types.PaginateReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}
	logrus.Infof("user id: %v", userId)

	query := db.DB.RenewalAgreement.Query().Where(renewalagreement.DeletedAt(utils.ZeroTime))

	total, err := query.Count(c)
	if err != nil {
		logrus.Errorf("db get renewal agreement total count failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	renewalAgreements, err := query.Offset(req.PageSize * (req.PageIndex - 1)).Limit(req.PageSize).All(c)
	if err != nil {
		logrus.Errorf("db get renewal agreement list failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	var resp common_types.PaginateResp
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = total
	resp.List = renewalAgreements

	response.RespSuccess(c, resp)
}

// GetRenewalAgreementMission 获取任务协议详情
func GetRenewalAgreementMission(c *gin.Context) {
	var req types.GetRenewalAgreementMissionReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}
	logrus.Infof("user id: %v", userId)

	renewalAgreement, err := db.DB.RenewalAgreement.Query().Where(renewalagreement.DeletedAt(utils.ZeroTime)).Where(renewalagreement.ID(req.ID)).First(c)
	if cep_ent.IsNotFound(err) {
		logrus.Warningf("renewal agreement id(%v) not exist", req.ID)
		response.RespError(c, code.NotFound)
		return
	} else if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, renewalAgreement)
}

// AddRenewalAgreementMission 新增任务协议（开通自动续费）
func AddRenewalAgreementMission(c *gin.Context) {
	var req types.AddRenewalAgreementMissionReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	missionID, err := strconv.ParseInt(req.MissionID, 10, 64)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, "missionID 需为全数字")
		return
	}

	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}
	logrus.Infof("user id: %v", userId)

	// 查询用户余额是否足够（目前设定需大于 20000cep）
	userWallet, err := db.DB.Wallet.Query().Where(wallet.UserID(userId), wallet.SymbolID(types.CepSymbolID)).First(c)
	if cep_ent.IsNotFound(err) {
		logrus.Warningf("user(%v) not have cep wallet", userId)
		response.RespErrorWithMsg(c, code.NotFound, "用户没有 cep")
		return
	} else if err != nil {
		logrus.Errorf("db get user wallet failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	if userWallet.Amount < 20000 {
		response.RespError(c, code.NotEnough)
		return
	}

	// 查询该用户是否有该任务的正在进行中的自动续费协议
	exist, err := db.DB.RenewalAgreement.Query().Where(renewalagreement.DeletedAt(utils.ZeroTime)).Where(renewalagreement.UserID(userId), renewalagreement.MissionID(missionID), renewalagreement.SubStatusEQ(enums.RenewalSubStatusSubscribing)).Exist(c)
	if err != nil {
		logrus.Errorf("db query exist renewal agreement failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	// 该任务已经存在自动续费的协议
	if exist {
		response.RespError(c, code.SourceExist)
		return
	}

	// 查询到任务，获取任务到期时间
	//_mission, err := db.DB.Mission.Query().Where(mission.DeletedAt(utils.ZeroTime), mission.ID(missionID), mission.StateEQ(enums.MissionStateSupplying)).First(c)
	_mission, err := db.DB.Mission.Query().Where(mission.DeletedAt(utils.ZeroTime), mission.ID(missionID)).First(c)
	if cep_ent.IsNotFound(err) {
		response.RespErrorWithMsg(c, code.NotFound, "任务启动中，请等待任务启动结束！")
		return
	} else if err != nil {
		logrus.Errorf("db query mission(%d) failed, err: %v", missionID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	// 该任务不存在正在进行的自动续费协议，添加
	create := db.DB.RenewalAgreement.Create().
		SetUserID(userId).
		SetMissionID(missionID).
		SetType(enums.RenewalType(req.Type)).
		SetSubStatus(enums.RenewalSubStatusSubscribing).
		SetSymbolID(types.CepSymbolID).
		SetNextPayTime(*_mission.ExpiredAt).
		SetPayStatus(enums.RenewalPayStatusSucceed) // todo：新开的协议支付状态待确认
	if req.FirstPay > 0 {
		create = create.SetFirstPay(req.FirstPay)
	}
	if req.AfterPay > 0 {
		create = create.SetAfterPay(req.AfterPay)
	}
	if req.NextPayTime != nil {
		create = create.SetNextPayTime(*req.NextPayTime)
	}
	renewalAgreement, err := create.Save(c)
	if err != nil {
		logrus.Errorf("db create renewal agreement failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, renewalAgreement)
}

// AddRenewalAgreementMissionBatch 批量新增任务协议（批量开通自动续费）
func AddRenewalAgreementMissionBatch(c *gin.Context) {
	var req types.AddRenewalAgreementMissionBatchReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}
	logrus.Infof("user id: %v", userId)

	// 查询用户余额是否足够（目前设定需大于 20000cep）
	userWallet, err := db.DB.Wallet.Query().Where(wallet.UserID(userId), wallet.SymbolID(types.CepSymbolID)).First(c)
	if cep_ent.IsNotFound(err) {
		logrus.Warningf("user(%v) not have cep wallet", userId)
		response.RespErrorWithMsg(c, code.NotFound, "用户没有 cep")
		return
	} else if err != nil {
		logrus.Errorf("db get user wallet failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	if userWallet.Amount < 20000 {
		response.RespError(c, code.NotEnough)
		return
	}

	var renewalAgreementCreates []*cep_ent.RenewalAgreementCreate
	for _, missionIDStr := range req.MissionIDs {
		missionID, err := strconv.ParseInt(missionIDStr, 10, 64)
		if err != nil {
			response.RespErrorWithMsg(c, code.InvalidParams, "missionID 需为全数字")
			return
		}
		// 查询该用户是否有该任务的自动续费协议
		exist, err := db.DB.RenewalAgreement.Query().Where(renewalagreement.DeletedAt(utils.ZeroTime)).Where(renewalagreement.UserID(userId), renewalagreement.MissionID(missionID), renewalagreement.SubStatusEQ(enums.RenewalSubStatusSubscribing)).Exist(c)
		if err != nil {
			logrus.Errorf("db query exist renewal agreement failed, err: %v", err)
			response.RespError(c, code.ServerErrDB)
			return
		}
		// 该任务已经存在自动续费的协议
		if exist {
			response.RespError(c, code.SourceExist)
			return
		}

		// 查询到任务，获取任务到期时间
		_mission, err := db.DB.Mission.Query().Where(mission.DeletedAt(utils.ZeroTime), mission.ID(missionID)).First(c)
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		} else if err != nil {
			logrus.Errorf("db query mission(%d) failed, err: %v", missionID, err)
			response.RespError(c, code.ServerErrDB)
			return
		}

		// 该任务不存在自动续费协议，添加
		create := db.DB.RenewalAgreement.Create().
			SetUserID(userId).
			SetMissionID(missionID).
			SetType(enums.RenewalType(req.Type)).
			SetSubStatus(enums.RenewalSubStatusSubscribing).
			SetSymbolID(types.CepSymbolID).
			SetNextPayTime(*_mission.ExpiredAt).
			SetPayStatus(enums.RenewalPayStatusSucceed) // todo：新开的协议支付状态待确认
		if req.FirstPay > 0 {
			create = create.SetFirstPay(req.FirstPay)
		}
		if req.AfterPay > 0 {
			create = create.SetAfterPay(req.AfterPay)
		}
		if req.NextPayTime != nil {
			create = create.SetNextPayTime(*req.NextPayTime)
		}
		renewalAgreementCreates = append(renewalAgreementCreates, create)
	}
	// 批量创建
	renewalAgreements, err := db.DB.RenewalAgreement.CreateBulk(renewalAgreementCreates...).Save(c)
	if err != nil {
		logrus.Errorf("db create renewal agreement batch failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, renewalAgreements)
}

// DeleteRenewalAgreementMission 删除任务协议（取消自动续费）
func DeleteRenewalAgreementMission(c *gin.Context) {
	var req types.DeleteRenewalAgreementMissionReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}
	logrus.Infof("user id: %v", userId)

	renewalAgreement, err := db.DB.RenewalAgreement.UpdateOneID(req.ID).SetSubStatus(enums.RenewalSubStatusFinished).SetDeletedAt(time.Now()).Save(c)
	if cep_ent.IsNotFound(err) {
		response.RespError(c, code.NotFound)
		return
	} else if err != nil {
		logrus.Errorf("db delete renewal agreement failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, renewalAgreement)
}
