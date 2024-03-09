package controllers

import (
	"entgo.io/ent/dialect/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/common"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/mission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/missionkind"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/missionorder"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/predicate"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/renewalagreement"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/response"
	"slices"
	"time"
	"vpay/internal/db"
	"vpay/internal/types"
)

// ListMission 任务列表
func ListMission(c *gin.Context) {
	var req types.ListMissionReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	logrus.Debugf("req: %+v", req)

	// 用户凭证拿到 user_id
	userID, err := getUserId(c)
	if err != nil {
		return
	}
	if !req.FrontType.IsExist() || !req.FrontWay.IsExist() || !req.FrontMissionBillingType.IsExist() {
		response.RespErrorInvalidParams(c, errors.New("前端传入不合法参数"))
		return
	}

	// 拿到任务类型
	missionTypeSet := types.InitFrontMissionTypeSet()
	missionTypes := missionTypeSet.MissionTypeIntersection(string(req.FrontType), string(req.FrontWay))
	// 拿到状态类型
	missionStateSet := types.InitFrontMissionState()
	var missionStates []types.StateStruct
	for i, _ := range req.FrontState {
		if !req.FrontState[i].IsExist() {
			response.RespErrorInvalidParams(c, errors.New("前端传入不合法参数"))
			return
		}
		missionStates = append(missionStates, missionStateSet.Values(string(req.FrontState[i]))...)
	}

	// 拿到计费方式
	missionBillingTypeSet := types.InitFrontMissionBillingTypeSet()
	missionBillingTypes := missionBillingTypeSet.Values(string(req.FrontMissionBillingType))
	// 开始查询
	query := db.DB.Mission.Query().
		Where(mission.DeletedAt(common.ZeroTime)).
		Where(mission.UserID(userID)).
		Where(mission.HasMissionKindWith(missionkind.BillingTypeIn(missionBillingTypes...))).
		WithMissionOrders(func(query *cep_ent.MissionOrderQuery) {
			query.Order(cep_ent.Desc(missionorder.FieldCreatedAt))
		}).
		WithRenewalAgreements(func(query *cep_ent.RenewalAgreementQuery) {
			query.Where(renewalagreement.SubStatusEQ(enums.RenewalSubStatusSubscribing))
		})
	if req.ID != 0 {
		query = query.Where(mission.ID(req.ID))
	}
	// 查询条件：任务类型
	query = query.Where(mission.TypeIn(missionTypes...))
	// 查询条件：任务状态
	var options []predicate.Mission
	for i, _ := range missionStates {
		options = append(options, mission.And(
			mission.StateEQ(
				missionStates[i].MissionState),
			mission.TypeIn(
				missionTypeSet.MissionTypeIntersection(
					string(types.WayNIL), string(missionStates[i].FrontWay))...,
			),
		))
	}

	query = query.Where(
		mission.Or(
			options...,
		),
	)
	// 查询条件：应用时间区间
	if req.StartedAt != nil && req.FinishedAt != nil && req.StartedAt.Before(*req.FinishedAt) {
		query.Where(mission.StartedAtGTE(*req.StartedAt), mission.FinishedAtLTE(*req.FinishedAt))
	}
	//// 查询条件：是否排序积分消耗
	//if req.DescCep != nil && *req.DescCep == true {
	//	query = query.Order(cep_ent.Desc(mission.FieldUnitCep))
	//}
	query = query.Order(func(selector *sql.Selector) {
		//selector.OrderExpr(sql.Expr("(case state when 'supplying' then 7 when 'doing' then 6 when 'waiting' then 5 when 'closing' then 4 when 'succeed' then 3 when 'failed' then 3 when 'canceled' then 3 else 0 end)desc"))
		selector.OrderExpr(sql.Expr("(case state when 'supplying' then 7 when 'doing' then 6 when 'waiting' then 5 when 'closing' then 4 else 0 end)desc"))

	})
	query = query.Order(cep_ent.Desc(mission.FieldFreeAt)).Order(cep_ent.Desc(mission.FieldStartedAt))
	// 查询条件：是否排序时间
	switch {
	case req.DescFinishedAt != nil && *req.DescFinishedAt == true:
		query = query.Order(cep_ent.Desc(mission.FieldFinishedAt))
	case req.DescStartedAt != nil && *req.DescStartedAt == true:
		query = query.Order(cep_ent.Desc(mission.FieldStartedAt))
	case req.DescExpiredAt != nil && *req.DescExpiredAt == true:
		query = query.Order(cep_ent.Desc(mission.FieldExpiredAt))
	case req.DescFinishedAt != nil && *req.DescFinishedAt == false:
		query = query.Order(cep_ent.Asc(mission.FieldFinishedAt))
	case req.DescStartedAt != nil && *req.DescStartedAt == false:
		query = query.Order(cep_ent.Asc(mission.FieldStartedAt))
	case req.DescExpiredAt != nil && *req.DescExpiredAt == false:
		query = query.Order(cep_ent.Asc(mission.FieldExpiredAt))
	default:
		query = query.Order(cep_ent.Desc(mission.FieldCreatedAt))
	}

	// select 和 count 不搭
	countQuery := query.Clone()
	selectMissionColumns := slices.DeleteFunc(mission.Columns, func(s string) bool {
		switch s {
		case mission.FieldBody,
			mission.FieldCallBackURL,
			mission.FieldCallBackInfo,
			mission.FieldCallBackData,
			mission.FieldCreatedBy,
			mission.FieldUpdatedBy,
			mission.FieldBlackDeviceIds,
			mission.FieldWhiteDeviceIds,
			mission.FieldInnerMethod,
			mission.FieldInnerURI,
			mission.FieldRespBody,
			mission.FieldRespStatusCode:
			return true
		default:
			return false
		}
	})

	total, err := countQuery.Count(c)
	if err != nil {
		logrus.Errorf("db query mission count failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	missions, err := query.Select(selectMissionColumns...).Offset(req.PageSize * (req.PageIndex - 1)).Limit(req.PageSize).All(c)

	if err != nil {
		logrus.Errorf("db query missions pagination failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	frontType := types.InitFrontAppType()
	frontWay := types.InitFrontWay()
	var missionsResp = make([]MissionResp, len(missions))
	for idx, _mission := range missions {
		var amount int64
		// isNewDevice := true
		for _, _order := range _mission.Edges.MissionOrders {
			amount = amount + _order.TotalAmount
		}
		missionResp := MissionResp{
			Mission:      _mission,
			MissionType:  frontType[_mission.Type],
			MissionWay:   frontWay[_mission.Type],
			TotalConsume: amount,
		}

		if len(_mission.Edges.MissionOrders) >= 1 {
			missionResp.BillingType = string(_mission.Edges.MissionOrders[0].MissionBillingType)
			missionResp.BuyDuration = _mission.Edges.MissionOrders[0].BuyDuration
			// 判断是否是新机器
			//isNewDevice, err = myredis.Client.SIsMember(c, types.RedisNewDeviceSet, _mission.Edges.MissionOrders[0].DeviceID).Result()
			//if err != nil {
			//	logrus.Errorf("query redis to judge new device, err: %+v", err)
			//	response.RespError(c, code.ServerErrCache)
			//	return
			//}
		}
		missionResp.NewDevice = true
		//missionsResp = append(missionsResp, missionResp)
		missionsResp[idx] = missionResp
	}
	var resp common_types.PaginateResp
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = total
	resp.List = missionsResp

	response.RespSuccess(c, resp)
}

type MissionResp struct {
	*cep_ent.Mission
	MissionType  string `json:"mission_type"`
	BillingType  string `json:"billing_type"`
	BuyDuration  int64  `json:"buy_duration"`
	MissionWay   string `json:"mission_way"`
	TotalConsume int64  `json:"total_consume"`
	NewDevice    bool   `json:"new_device"`
}

// GetMission 任务详情
func GetMission(c *gin.Context) {
	var req types.PathIDReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	//missionID, err := strconv.ParseInt(req.ID, 10, 64)
	//if err != nil {
	//	response.RespErrorWithMsg(c, code.InvalidParams, "ID 格式错误")
	//	return
	//}
	missionID := req.ID

	_mission, err := db.DB.Mission.Query().WithMissionOrders(func(missionOrderQuery *cep_ent.MissionOrderQuery) {
		missionOrderQuery.Where(missionorder.DeletedAt(common.ZeroTime))
	}).Where(mission.DeletedAt(common.ZeroTime)).Where(mission.ID(missionID)).First(c)
	if cep_ent.IsNotFound(err) {
		response.RespError(c, code.NotFound)
		return
	} else if err != nil {
		logrus.Errorf("db query mission failed, err:%v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, _mission)
}

// DeleteMission 删除任务
func DeleteMission(c *gin.Context) {
	// 用户凭证拿到 user_id
	userID, err := getUserId(c)
	if err != nil {
		return
	}

	var req types.PathIDReq
	if err = c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("user id: %d, req: %+v", userID, req)

	// 判断该任务是不是属于该用户
	oldMission, err := db.DB.Mission.Query().Where(mission.ID(req.ID)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		}
		logrus.Errorf("db query mission failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	if oldMission.UserID != userID {
		response.RespErrorWithMsg(c, code.NotEnough, "权限不足")
		return
	}

	_mission, err := db.DB.Mission.UpdateOne(oldMission).SetDeletedAt(time.Now()).SetFreeAt(time.Now()).Save(c)
	if err != nil {
		logrus.Errorf("db update mission failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, _mission)
}
