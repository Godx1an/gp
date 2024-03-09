package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/cdkinfo"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/response"
	"time"
	"vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/logic"
	"vpay/internal/types"
	"vpay/utils"
	"vpay/utils/db_utils"
)

// ListCDK 分页获取 cdk 数据
func ListCDK(c *gin.Context) {
	var req types.ListCDKReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	if req.TempAuth != types.TempAuth {
		response.RespError(c, code.AuthFailed)
		return
	}

	query := db.DB.CDKInfo.Query().WithIssueUser().WithUseUser().Where(cdkinfo.DeletedAt(utils.ZeroTime)).Order(cep_ent.Desc(cdkinfo.FieldCreatedAt))
	if req.Type != "" {
		query = query.Where(cdkinfo.TypeEQ(req.Type))
	}
	if req.CdkNumber != "" {
		query = query.Where(cdkinfo.CdkNumberContains(req.CdkNumber))
	}

	total, err := query.Count(c)
	if err != nil {
		logrus.Errorf("db get cdk info count failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	cdkInfos, err := query.Offset((req.PageIndex - 1) * req.PageSize).Limit(req.PageSize).All(c)
	if err != nil {
		logrus.Errorf("db get cdk info list failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	var resp common_types.PaginateResp
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = total
	resp.List = cdkInfos

	response.RespSuccess(c, resp)
}

// GetCDK 获取 cdk 详情
func GetCDK(c *gin.Context) {
	var req types.PathIDReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	cdkInfo, err := db.DB.CDKInfo.Query().WithIssueUser().WithUseUser().Where(cdkinfo.DeletedAt(utils.ZeroTime), cdkinfo.ID(req.ID)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		}
		logrus.Errorf("db get cdk info by id failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, cdkInfo)
}

// GetCDKDetailByParams 根据唯一参数获取 cdk 详情
func GetCDKDetailByParams(c *gin.Context) {
	var req types.DetailCDKReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	cdkInfo, err := db.DB.CDKInfo.Query().WithIssueUser().WithUseUser().Where(cdkinfo.DeletedAt(utils.ZeroTime), cdkinfo.CdkNumber(req.CDKNumber)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		}
		logrus.Errorf("db get cdk info by id failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, cdkInfo)
}

// CreateCDK 创建 cdk
func CreateCDK(c *gin.Context) {
	// fixme: 暂时放开权限
	//userID, err := common.GetUserId(c)
	//if err != nil {
	//	response.DynamicRespErr(c, err)
	//	return
	//}
	var (
		req    types.CreateCDKReq
		err    error
		userID = types.GenesisUserID
	)
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	if req.TempAuth != types.TempAuth {
		response.RespError(c, code.AuthFailed)
		return
	}

	var cdkInfos = make([]*cep_ent.CDKInfo, 0)
	if err = db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		// 批量创建
		var cdkCreates = make([]*cep_ent.CDKInfoCreate, 0)
		for i := 0; i < req.GenerateAmount; i++ {
			// 生成 CDK 字符串
			cdkNumber := utils.GenerateRandomStringWithLength(types.CDKLength)
			create := tx.CDKInfo.Create().
				SetIssueUserID(userID).
				SetCdkNumber(cdkNumber).
				SetType(req.Type).
				SetGetCep(req.GetCepAmount).
				SetGetTime(req.GetDuration).
				SetStatus(enums.CDKStatusNormal)
			if req.Type == enums.CDKTypeGetGpuUse {
				create = create.SetBillingType(req.GetGpuBillingType)
			}
			cdkCreates = append(cdkCreates, create)
		}
		// 创建
		cdkInfos, err = tx.CDKInfo.CreateBulk(cdkCreates...).Save(c)
		if err != nil {
			logrus.Errorf("db batch create cdkinfo failed, err: %v", err)
			return err
		}

		return nil
	}); err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, cdkInfos)
}

// UpdateCDK 更新 cdk
func UpdateCDK(c *gin.Context) {
	var req types.UpdateCDKReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	response.RespSuccess(c, nil)
}

// DeleteCDK 删除 cdk
func DeleteCDK(c *gin.Context) {
	var req types.PathIDReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	cdkInfo, err := db.DB.CDKInfo.UpdateOneID(req.ID).SetDeletedAt(time.Now()).Save(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		}
		logrus.Errorf("db delete cdk info(%d) failed, err: %v", req.ID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, cdkInfo)
}

// UseCDK 使用 cdk
func UseCDK(c *gin.Context) {
	userID, err := common.GetUserId(c)
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}

	var req types.UseCDKReq
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 查询到 cdk 数据
	cdkInfo, err := db.DB.CDKInfo.Query().Where(cdkinfo.DeletedAt(utils.ZeroTime), cdkinfo.CdkNumber(req.CDKNumber)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		}
		logrus.Errorf("db get cdk info by id failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	if cdkInfo.Status != enums.CDKStatusNormal || cdkInfo.UseUserID != 0 || (cdkInfo.UsedAt != nil && !cdkInfo.UsedAt.IsZero()) {
		// 状态不是正常 或 使用者 ID 不是 0 或 （被使用时间不是空 且 被使用时间不是零时间）
		response.RespErrorWithMsg(c, code.InvalidKey, "CDK 已被使用或已失效")
		return
	}

	// 使用 cdk 功能
	if err = db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		// 判断 cdk 的类型，做不同的处理
		switch cdkInfo.Type {
		case enums.CDKTypeGetCep:
			// 兑换脑力值，CDK 状态等数据变更，使用该 CDK 的用户脑力值增加
			if err = tx.CDKInfo.UpdateOne(cdkInfo).SetStatus(enums.CDKStatusUsed).SetUseUserID(userID).SetUsedAt(time.Now()).Exec(c); err != nil {
				logrus.Errorf("use cdk db update cdk date failed, err: %v", err)
				response.RespError(c, code.ServerErrDB)
				return err
			}
			// 新增流水, 用户钱包余额增加
			if _, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
				Tx:           tx,
				TargetUserID: userID,
				SourceUserID: types.GenesisUserID,
				SymbolID:     types.CepSymbolID,
				Amount:       cdkInfo.GetCep,
				Type:         enums.BillTypeCdk,
				Way:          enums.BillWayCdkExchange,
			}); err != nil {
				logrus.Errorf("gen bill and update wallet failed, err: %v", err)
				response.RespError(c, code.ServerErr)
				return err
			}
		case enums.CDKTypeGetGpuUse:
			// 兑换 gpu 使用权
			response.RespErrorWithMsg(c, code.NotEnough, "gpu 使用权兑换功能暂未开通")
			return errors.New("gpu 使用权兑换功能暂未开通")
		default:
			response.RespErrorWithMsg(c, code.InvalidKey, "非法类型的 CDK")
			return errors.New("非法类型的 CDK")
		}
		return nil
	}); err != nil {
		return
	}

	response.RespSuccess(c, response.EmptyData)
}
