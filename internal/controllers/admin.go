package controllers

import (
	"context"
	"entgo.io/ent/dialect/sql"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/jinzhu/copier"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/common"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/bill"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/devicegpumission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/invite"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/lotto"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/lottochancerule"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/lottoprize"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/mission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/missionkind"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/missionorder"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/renewalagreement"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/transferorder"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/wallet"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/response"
	"regexp"
	"strconv"
	"time"
	_common "vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/handlers"
	"vpay/internal/myredis"
	"vpay/internal/types"
	"vpay/utils/db_utils"
)

const (
	PrizeStatusUnknown  lottoprize.Status = "unknow"
	PrizeStatusNormal   lottoprize.Status = "normal"
	PrizeStatusCanceled lottoprize.Status = "canceled"
)

type UsersTreeReq struct {
	Info      string     `form:"info" binding:"required"`
	StartData *time.Time `form:"start_data"`
	EndData   *time.Time `form:"end_data"`
	common_types.PaginateReq
}

type UsersTreeResp struct {
	*cep_ent.User
	TreeEdge                    `json:"tree_edge"`
	TotalInvite                 int    `json:"total_invite"`                   // 总邀请人数
	StraightInvite              int    `json:"straight_invite"`                // 直接邀请人数
	ManTotalRechargeMoney       int64  `json:"man_total_recharge_money"`       // 个人充值总金额
	TotalRechargeFrequency      int    `json:"total_recharge_frequency"`       // 个人充值次数
	InviteCode                  string `json:"invite_code"`                    // 邀请码
	InviteRechargeMoney         int64  `json:"invite_recharge_money"`          // 邀请人充值金额
	StraightInviteRechargeMoney int64  `json:"straight_invite_recharge_money"` // 直接邀请人充值金额
	TotalRechargeMoney          int64  `json:"total_recharge_money"`           // 邀请人充值金额 + 个人充值金额
}
type TreeEdge struct {
	Child []*UsersTreeResp `json:"child,omitempty"`
}

// UsersTree 展示某个用户的所有下级关系
func UsersTree(c *gin.Context) {
	var req UsersTreeReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	// 手机号查询第一个人
	topUser, err := db.DB.User.Query().Where(user.Phone(req.Info), user.DeletedAt(common.ZeroTime)).WithInvites().First(c)
	if cep_ent.IsNotFound(err) {
		topUser, err = db.DB.User.Query().Where(user.HasInvitesWith(invite.InviteCodeEQ(req.Info))).First(c)
		if err != nil {
			response.DynamicRespErr(c, err)
			return
		}
	} else if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	// 广度优先算法 BFS，递归探树底
	tempDepthUserMap := make(map[int64]*UsersTreeResp)
	userTreeRoot := &UsersTreeResp{
		User: topUser,
		TreeEdge: TreeEdge{
			Child: make([]*UsersTreeResp, 0),
		},
		TotalInvite: 0,
	}
	tempDepthUserMap[topUser.ID] = userTreeRoot
	tempDepthUserIDs := []int64{topUser.ID}
	// 开始递归
	if err = fetchUserTreeUnit(c, tempDepthUserMap, tempDepthUserIDs); err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	if (req.StartData != nil && req.EndData == nil) || (req.StartData == nil && req.EndData != nil) {
		response.RespErrorInvalidParams(c, errors.New("缺失时间参数，请同时设置开始时间和结束时间"))
	}
	for _, child := range userTreeRoot.TreeEdge.Child {
		userTreeRoot.StraightInviteRechargeMoney += child.ManTotalRechargeMoney
	}
	if req.StartData != nil && req.EndData != nil {

		for i := 0; i < len(userTreeRoot.TreeEdge.Child); i++ {
			if !userTreeRoot.TreeEdge.Child[i].CreatedAt.Before(*req.EndData) ||
				!userTreeRoot.TreeEdge.Child[i].CreatedAt.After(*req.StartData) {
				if i == len(userTreeRoot.TreeEdge.Child)-1 {
					userTreeRoot.TreeEdge.Child = userTreeRoot.TreeEdge.Child[:i]

				} else {
					userTreeRoot.TreeEdge.Child = append(userTreeRoot.TreeEdge.Child[:i], userTreeRoot.TreeEdge.Child[i+1:]...)
				}
				i--
			}
		}
	}
	if len(userTreeRoot.TreeEdge.Child) < req.PageSize*(req.PageIndex-1) {
		userTreeRoot.TreeEdge = TreeEdge{}
	} else if len(userTreeRoot.TreeEdge.Child) < req.PageSize*req.PageIndex {
		userTreeRoot.TreeEdge.Child = userTreeRoot.TreeEdge.Child[req.PageSize*(req.PageIndex-1) : len(userTreeRoot.TreeEdge.Child)]
	} else {
		userTreeRoot.TreeEdge.Child = userTreeRoot.TreeEdge.Child[req.PageSize*(req.PageIndex-1) : req.PageSize*req.PageIndex]
	}

	var resp common_types.PaginateResp
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = len(userTreeRoot.TreeEdge.Child)
	resp.List = userTreeRoot
	response.RespSuccess(c, resp)
}

// 把当前的用户们的子用户一次性查出来（节省数据查询次数），再用逻辑自己分配好父子关系
func fetchUserTreeUnit(ctx context.Context, tempDepthUserMap map[int64]*UsersTreeResp, tempDepthUserIDs []int64) error {
	childUsers, err := db.DB.User.Query().
		Where(user.DeletedAt(common.ZeroTime), user.ParentIDIn(tempDepthUserIDs...)).
		Order(cep_ent.Desc(user.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return err
	}
	indexDepthUserIDs := make([]int64, len(tempDepthUserIDs))
	copy(indexDepthUserIDs, tempDepthUserIDs)
	// 这一层的 userIDs 已经没用了，清空后给新的一层使用
	tempDepthUserIDs = tempDepthUserIDs[:0]
	// 准备新的一层 userMap，等分配完成后就替换变量
	newTempDepthUserMap := make(map[int64]*UsersTreeResp)

	for _, childUser := range childUsers {
		tempDepthUserIDs = append(tempDepthUserIDs, childUser.ID)
		childNode := &UsersTreeResp{
			User: childUser,
			TreeEdge: TreeEdge{
				Child: make([]*UsersTreeResp, 0),
			},
			TotalInvite: 0,
		}
		newTempDepthUserMap[childUser.ID] = childNode
		// 用 map 快速找到各自的 parent
		tempParentUser, ok := tempDepthUserMap[childUser.ParentID]
		if !ok {
			return fmt.Errorf("WTF no parent for this user %+v", childUser)
		}
		tempParentUser.TreeEdge.Child = append(tempParentUser.TreeEdge.Child, childNode)

	}

	// BFS
	if len(tempDepthUserIDs) != 0 {
		if err = fetchUserTreeUnit(ctx, newTempDepthUserMap, tempDepthUserIDs); err != nil {
			return err
		}
		for _, userID := range indexDepthUserIDs {
			tempParentUser, ok := tempDepthUserMap[userID]
			if !ok {
				return fmt.Errorf("no info for this user %+v", tempParentUser)
			}
			tempParentUser.StraightInvite = len(tempParentUser.TreeEdge.Child)
			allRechargeBills, err := db.DB.Bill.Query().Where(bill.TargetUserID(tempParentUser.ID), bill.TypeEQ(enums.BillTypeRecharge)).All(ctx)
			if err != nil {
				return err
			}
			// 设置个人充值金额
			for _, _bill := range allRechargeBills {
				tempParentUser.ManTotalRechargeMoney = tempParentUser.ManTotalRechargeMoney + _bill.Amount
			}
			// 设置个人充值次数
			tempParentUser.TotalRechargeFrequency = len(allRechargeBills)
			// 设置邀请码
			inviteInfo, err := db.DB.Invite.Query().Where(invite.UserID(tempParentUser.ID)).First(ctx)
			if err != nil && !cep_ent.IsNotFound(err) {
				return err
			} else if cep_ent.IsNotFound(err) {
				tempParentUser.InviteCode = ""
			} else {
				tempParentUser.InviteCode = inviteInfo.InviteCode
			}
			// 设置邀请人充值金额
			for _, child := range tempParentUser.TreeEdge.Child {
				tempParentUser.InviteRechargeMoney = tempParentUser.InviteRechargeMoney + child.TotalRechargeMoney
			}
			// 设置邀请人总人数
			tempParentUser.TotalInvite = len(tempParentUser.TreeEdge.Child)
			for _, child := range tempParentUser.TreeEdge.Child {
				tempParentUser.TotalInvite += child.TotalInvite
			}
			// 设置邀请人充值金额+本人充值总金额
			tempParentUser.TotalRechargeMoney = tempParentUser.InviteRechargeMoney + tempParentUser.ManTotalRechargeMoney
		}
	} else {
		return nil
	}

	return nil
}

type ManualTransferReq struct {
	TargetUserID    int64  `json:"target_user_id,string"`
	TargetUserPhone string `json:"target_user_phone"`
	SourceUserID    int64  `json:"source_user_id,string"`
	SourceUserPhone string `json:"source_user_phone"`
	SymbolID        int64  `json:"symbol_id,string"`
	Amount          int64  `json:"amount" binding:"required"`
}

// ManualTransfer 手动转账
func ManualTransfer(c *gin.Context) {
	var req ManualTransferReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	// 手动转账目标用户
	var targetUserID int64
	// 先看用户 id
	if req.TargetUserID == 0 {
		// 没有再看用户手机号
		if req.TargetUserPhone == "" {
			// 再没有就是使用起源用户
			targetUserID = types.GenesisUserID
		} else {
			targetUser, err := db.DB.User.Query().Where(user.DeletedAt(common.ZeroTime), user.Phone(req.TargetUserPhone)).First(c)
			if err != nil {
				response.RespErrorInvalidParams(c, err)
				return
			}
			targetUserID = targetUser.ID
		}
	} else {
		targetUserID = req.TargetUserID
	}
	// 手动转账发起用户
	var sourceUserID int64
	// 先看用户 id
	if req.SourceUserID == 0 {
		// 没有在看用户手机号
		if req.SourceUserPhone == "" {
			// 再没有就是使用启用用户
			sourceUserID = types.GenesisUserID
		} else {
			// 只有手机号就查出用户 id
			sourceUser, err := db.DB.User.Query().Where(user.DeletedAt(common.ZeroTime), user.Phone(req.SourceUserPhone)).First(c)
			if err != nil {
				response.RespErrorInvalidParams(c, err)
				return
			}
			sourceUserID = sourceUser.ID
		}
	} else {
		sourceUserID = req.SourceUserID
	}

	// 没传就是默认货币 CEP
	if req.SymbolID == 0 {
		req.SymbolID = types.CepSymbolID
	}
	transferOrder, err := handlers.CreateManualTransferOrder(c, &handlers.CreateManualTransferOrderOptions{
		Tx:           nil,
		SymbolID:     req.SymbolID,
		Amount:       req.Amount,
		TargetUserID: targetUserID,
		SourceUserID: sourceUserID,
	})

	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	response.RespSuccess(c, transferOrder)
}

type SearchManualTransferBillsReq struct {
	common_types.PaginateReq
}

func SearchManualTransferBills(c *gin.Context) {
	var req SearchManualTransferBillsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	query := db.DB.TransferOrder.Query().
		Where(transferorder.TypeEQ(enums.TransferOrderTypeManual))
	totalCount, err := query.Count(c)
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	transferOrders, err := query.
		Order(cep_ent.Desc(transferorder.FieldCreatedAt)).
		WithBills().
		Offset((req.PageIndex - 1) * req.PageSize).
		Limit(req.PageSize).
		All(c)
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}

	response.RespSuccess(c, common_types.PaginateResp{
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
		Total:     totalCount,
		List:      transferOrders,
	})
	return
}

// SearchUserByPhone 根据手机号查询用户信息
func SearchUserByPhone(c *gin.Context) {
	var req struct {
		Phone string `form:"phone" json:"phone"`
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 查询用户
	_user, err := db.DB.User.Query().Where(user.PhoneEQ(req.Phone), user.DeletedAt(common.ZeroTime)).First(c)
	if cep_ent.IsNotFound(err) {
		logrus.Error(err)
		response.RespError(c, code.InvalidParams)
		return
	}
	if err != nil {
		logrus.Error(err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	var userInfo types.UserInfo
	userInfo.UserId = _user.ID
	userInfo.Phone = _user.Phone
	userInfo.RegisterTime = _user.CreatedAt
	userInfo.Phone = req.Phone

	if _user.ParentID != 0 {
		// 查询邀请人
		parentUser, err := db.DB.User.Query().Where(user.IDEQ(_user.ParentID)).First(c)
		if err != nil {
			logrus.Error(err)
			response.RespError(c, code.ServerErrDB)
			return
		}
		userInfo.ParentUserPhone = parentUser.Phone
		userInfo.ParentUserName = parentUser.Name
	}

	// 查询充值金额
	var v []struct {
		Sum, Count int64
	}
	query := db.DB.Bill.Query().Where(bill.TargetUserID(_user.ID), bill.SymbolID(1), bill.TypeEQ("recharge"), bill.DeletedAtEQ(common.ZeroTime))
	err = query.Aggregate(
		cep_ent.Sum(bill.FieldAmount),
		cep_ent.Count(),
	).Scan(c, &v)
	if err != nil {
		logrus.Error(err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	userInfo.RechargeTimes = v[0].Count
	userInfo.RechargeAmount = float64(v[0].Sum / 1000)

	// 查询余额
	_wallet, err := db.DB.Wallet.Query().
		Where(wallet.UserID(_user.ID), wallet.DeletedAtEQ(common.ZeroTime)).
		WithSymbol().
		All(c)
	if err != nil {
		logrus.Error(err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	userInfo.AccountBalance = _wallet
	response.RespSuccess(c, userInfo)
}

type RechargeRecordReq struct {
	Phone string `form:"phone" json:"phone" binding:"required"`
	common_types.PaginateReq
}

type RechargeRecordResp struct {
	Records []Records `json:"records"`
	common_types.PaginateResp
}

type Records struct {
	Cost float64       `json:"cost"`
	Bill *cep_ent.Bill `json:"bill"`
}

// ListRechargeRecord 查询充值记录
func ListRechargeRecord(ctx *gin.Context) {
	var req RechargeRecordReq
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	_user, err := db.DB.User.Query().Where(user.PhoneEQ(req.Phone)).First(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	query := db.DB.Bill.Query().
		Where(bill.DeletedAtEQ(common.ZeroTime),
			bill.TargetUserIDEQ(_user.ID),
			bill.TypeEQ(enums.BillTypeRecharge),
			bill.SymbolID(1)).Order(cep_ent.Desc(bill.FieldCreatedAt))
	total, err := query.Count(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	rechargeRecord, err := query.WithTransferOrder().
		Offset(req.PageSize * (req.PageIndex - 1)).
		Limit(req.PageSize).
		All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	var resp RechargeRecordResp
	for _, record := range rechargeRecord {
		var records Records
		temp := float64(record.Amount) / float64(1000)
		records.Bill = record
		records.Cost = temp
		resp.Records = append(resp.Records, records)
	}
	resp.Total = total
	resp.PageSize = req.PageSize
	resp.PageIndex = req.PageIndex
	response.RespSuccess(ctx, resp)
}

type AppRecordReq struct {
	Phone string `form:"phone" json:"phone" binding:"required"`
	common_types.PaginateReq
}

type AppRecordResp struct {
	common_types.PaginateResp
	UnionRecord []*unionRecord `json:"records"`
}

type unionRecord struct {
	UnionMission *cep_ent.Mission `json:"mission"`
	AppName      string           `json:"app_name"`
	Usage        string           `json:"usage"`
	BillingMode  string           `json:"billing_mode"`
	Consumption  int64            `json:"consumption"`
}

// ListAppRecord 查询应用记录
func ListAppRecord(ctx *gin.Context) {
	var req AppRecordReq
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	_user, err := db.DB.User.Query().Where(user.PhoneEQ(req.Phone)).First(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	query := db.DB.Mission.Query().Where(mission.UserIDEQ(_user.ID))
	total, err := query.Count(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	missions, err := query.WithMissionOrders().
		Order(cep_ent.Desc(mission.FieldCreatedAt)).
		Offset(req.PageSize * (req.PageIndex - 1)).
		Limit(req.PageSize).
		All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	frontWay := types.InitFrontWay()
	billingTypeSet := types.InitBillingTypeToFrontMissionMap()
	appTypeSet := types.InitFrontAppType()

	var resp AppRecordResp
	resp.Total = total
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	records := make([]*unionRecord, 0, len(missions))

	for _, _mission := range missions {
		var tempRecord unionRecord
		tempRecord.UnionMission = _mission
		tempRecord.AppName = appTypeSet[_mission.Type]
		tempRecord.BillingMode = billingTypeSet[_mission.Edges.MissionOrders[0].MissionBillingType]
		tempRecord.Usage = frontWay[_mission.Type]
		var freezeWalletsMoney int64
		// 获取该任务冻结的金额
		for _, missionOrder := range _mission.Edges.MissionOrders {
			missionOrderFreezeAmount, err := myredis.Client.Get(ctx, fmt.Sprintf(types.RedisKeyUserFreezeWalletsMoney, _user.ID, missionOrder.ID, missionOrder.SymbolID)).Int64()
			if err != nil {
				if err == redis.Nil {
					// 找不到，先按没冻结处理
					freezeWalletsMoney += 0
				} else {
					logrus.Errorf("redis get _user freeze money int64 failed, err: %v", err)
					continue
				}
			}
			freezeWalletsMoney += missionOrderFreezeAmount
		}
		// 获取任务已结算金额
		var tempAmount int64
		for _, unionOrder := range _mission.Edges.MissionOrders {
			tempAmount += unionOrder.TotalAmount
		}
		tempAmount += freezeWalletsMoney
		tempRecord.Consumption = tempAmount
		records = append(records, &tempRecord)
	}
	resp.UnionRecord = records
	response.RespSuccess(ctx, resp)
}

// AdminGetGpuInfo 查询GPU信息
func AdminGetGpuInfo(ctx *gin.Context) {

	var v2 []struct {
		GPUId     int64 `json:"gpu_id"`
		GPUNumber int64 `json:"count"`
	}

	// 获取每种GPU在线数量
	err := db.DB.DeviceGpuMission.
		Query().
		Where(devicegpumission.GpuStatusNEQ(enums.GpuStatusOffline)).
		Order(cep_ent.Desc(devicegpumission.FieldGpuID)).
		GroupBy(devicegpumission.FieldGpuID).
		Aggregate(cep_ent.As(cep_ent.Count(), "count")).
		Scan(ctx, &v2)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	var gpuInfo GPUInfoResp
	// 创建 GPU的ID 和返回GPU列表的下标映射关系
	GpuIDMap := make(map[int64]int)
	index := 0
	for _, v22 := range v2 {
		var gpu GpuUsage
		gpuID := v22.GPUId
		gpu.GpuID = gpuID
		gpu.GpuNum = v22.GPUNumber
		gpu.IsSoldOut = true
		gpuInfo.GpuInfo = append(gpuInfo.GpuInfo, &gpu)
		GpuIDMap[gpuID] = index
		index++
	}

	// 填入对应的GPU版本名称
	gpuVersion, err := db.DB.Gpu.Query().All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	for _, info := range gpuVersion {
		i, ok := GpuIDMap[info.ID]
		if ok {
			gpuInfo.GpuInfo[i].GpuVersion = info.Version
		}
	}

	var freeGpus []struct {
		freeGpuID int64
	}
	err = db.DB.DeviceGpuMission.Query().
		Where(devicegpumission.GpuStatusEQ(enums.GpuStatusFree)).
		Unique(true).
		Select(devicegpumission.FieldGpuID).Scan(ctx, &freeGpus)

	for _, freeGpu := range freeGpus {
		gpuInfo.GpuInfo[GpuIDMap[freeGpu.freeGpuID]].IsSoldOut = false
	}

	// 创建GPU版本和 下标的map
	gpuVersionMap := make(map[enums.GpuVersion]int)

	for i, union := range gpuInfo.GpuInfo {
		gpuVersionMap[union.GpuVersion] = i
	}

	// 查询正在工作中的 GPU
	gpus, err := db.DB.DeviceGpuMission.Query().
		Where(devicegpumission.GpuStatusEQ(enums.GpuStatusBusy)).
		All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	// 将每个 GPU 正在做的任务数加到大类当前应用数量中
	for _, gpu := range gpus {
		i, ok := GpuIDMap[gpu.GpuID]
		if ok {
			gpuInfo.GpuInfo[i].GpuUsed += int64(len(gpu.MissionID))
			if gpuInfo.GpuInfo[i].IsSoldOut && len(gpu.MissionID) < int(gpu.MaxOnlineMission) {
				gpuInfo.GpuInfo[i].IsSoldOut = false
			}
		}
	}

	// 初始化应用大类map
	appTypeMap := make(map[enums.MissionCategory]int64)
	var categories []struct {
		Category enums.MissionCategory
	}
	err = db.DB.MissionKind.Query().
		Unique(true).
		Select(missionkind.FieldCategory).Scan(ctx, &categories)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	for _, union := range categories {
		appTypeMap[union.Category] = 0
	}

	// 初始化任务数量和时间任务的 map
	for _, union := range gpuInfo.GpuInfo {
		err = copier.Copy(&union.AppTypes, appTypeMap)
		union.BillingMode = initBillingModeMap()
		if err != nil {
			logrus.Error("failed to copy map")
			return
		}
	}

	// 查询供应中的任务
	_missions, err := db.DB.Mission.Query().
		Where(mission.StateEQ(enums.MissionStateSupplying)).
		WithMissionKind().
		All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	// 统计供应中的应用类型
	for _, union := range _missions {
		i, ok := gpuVersionMap[union.GpuVersion]
		if ok {
			gpuInfo.GpuInfo[i].AppTypes[union.Edges.MissionKind.Category] += 1
		}
	}

	supplyingOrder, err := db.DB.MissionOrder.Query().
		Where(missionorder.StatusEQ(enums.MissionOrderStatusSupplying),
			missionorder.DeletedAtEQ(common.ZeroTime)).
		WithMission().
		All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	billingTypeMap := types.InitBillingTypeToFrontMissionMap()

	for _, union := range supplyingOrder {
		temp := billingTypeMap.Values(union.MissionBillingType)
		i, ok := gpuVersionMap[union.Edges.Mission.GpuVersion]
		if ok {
			_, isOk := gpuInfo.GpuInfo[i].BillingMode[temp]
			if isOk {
				gpuInfo.GpuInfo[i].BillingMode[temp] += 1
			}
		}
	}

	var waitingNum []struct {
		GpuVersion enums.GpuVersion `json:"gpu_version"`
		Count      int64            `json:"count"`
	}

	err = db.DB.Mission.
		Query().
		Where(mission.DeletedAtEQ(common.ZeroTime), mission.StatusEQ(enums.MissionStatusWaiting)).
		GroupBy(mission.FieldGpuVersion).
		Scan(ctx, &waitingNum)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	for _, union := range waitingNum {
		gpuInfo.GpuInfo[gpuVersionMap[union.GpuVersion]].WaitingNum = union.Count
	}

	for _, union := range gpuInfo.GpuInfo {
		union.BillingMode["time_plan_hour_to_week"] = union.BillingMode["time_plan_hour"] + union.BillingMode["time_plan_day"] + union.BillingMode["time_plan_week"]
	}

	// 查询所有等待中的应用
	mo, err := db.DB.MissionOrder.Query().Where(
		missionorder.StatusEQ(enums.MissionOrderStatusWaiting),
		missionorder.DeletedAtEQ(common.ZeroTime),
	).All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	queueSum := 0
	for _, _mission := range mo {
		if _mission.StartedAt.Sub(_mission.CreatedAt).Minutes() > 10 {
			queueSum++
		} else if _mission.StartedAt.Equal(common.ZeroTime) && time.Now().Sub(_mission.CreatedAt).Minutes() > 10 {
			queueSum++
		}
	}
	gpuInfo.QueuedItemNum = queueSum

	response.RespSuccess(ctx, gpuInfo)
}

func initBillingModeMap() map[string]int64 {
	tempBillModeMap := make(map[string]int64)
	tempBillModeMap["time_plan_day"] = 0
	tempBillModeMap["time_plan_hour"] = 0
	tempBillModeMap["time_plan_week"] = 0
	tempBillModeMap["time_plan_month"] = 0
	tempBillModeMap["volume"] = 0
	tempBillModeMap["time_plan_hour_to_week"] = 0
	return tempBillModeMap
}

type GPUInfoResp struct {
	GpuInfo       []*GpuUsage `json:"gpu_info"`
	QueuedItemNum int         `json:"queued_item_num"`
}

type GpuUsage struct {
	GpuID       int64                           `json:"gpu_id"`
	GpuVersion  enums.GpuVersion                `json:"gpu_version"`
	GpuNum      int64                           `json:"gpu_num"`
	GpuUsed     int64                           `json:"gpu_used"`
	IsSoldOut   bool                            `json:"is_sold_out"`
	AppTypes    map[enums.MissionCategory]int64 `json:"app_types"`
	BillingMode map[string]int64                `json:"billing_mode"`
	WaitingNum  int64                           `json:"waiting_num"`
}

// AdminGetQueueInfo 获取排队信息
func AdminGetQueueInfo(ctx *gin.Context) {

	var queueInfo QueueInfoResp

	waitingNum, err := db.DB.Mission.Query().Where(mission.StateEQ("waiting"), mission.DeletedAtEQ(common.ZeroTime)).Count(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	queueInfo.WaitingNum = waitingNum

	// 获取当前日期和时间
	currentTime := time.Now()
	// 获取今天的日期
	year, month, day := currentTime.Date()
	location, err := time.LoadLocation("Asia/Shanghai")
	today := time.Date(year, month, day, 0, 0, 0, 0, location)
	_missions, err := db.DB.MissionOrder.Query().Where(
		missionorder.CreatedAtGTE(today),
		missionorder.DeletedAtEQ(common.ZeroTime),
	).All(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	queueSum := 0
	for _, _mission := range _missions {
		if _mission.StartedAt.Sub(_mission.CreatedAt).Minutes() > 10 {
			queueSum++
		} else if _mission.StartedAt.Equal(common.ZeroTime) && time.Now().Sub(_mission.CreatedAt).Minutes() > 10 {
			queueSum++
		}
	}
	queueInfo.QueuedItemNum = queueSum
	response.RespSuccess(ctx, queueInfo)
}

type QueueInfoResp struct {
	WaitingNum    int `json:"waiting_num"`
	QueuedItemNum int `json:"queued_item_num"`
}

func IsURL(input string) bool {
	urlRegex := regexp.MustCompile(`^(https?|ftp):\/\/[^\s/$.?#].[^\s]*$`)
	return urlRegex.MatchString(input)
}

func IsID(input string) bool {
	regex := regexp.MustCompile(`^\d{19}$`)
	return regex.MatchString(input)
}

func AdminGetMissionInfo(ctx *gin.Context) {
	var req struct {
		Info string `form:"info"`
	}
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}

	mode := "%" + req.Info + "%"
	var mo *cep_ent.MissionOrder
	var err error
	if !IsID(req.Info) {
		_mission, err := db.DB.Mission.Query().Where(func(selector *sql.Selector) {
			selector.Where(sql.Like(mission.FieldUrls, mode))
		}, mission.StatusEQ(enums.MissionStatusSupplying)).First(ctx)
		if cep_ent.IsNotFound(err) {
			logrus.Error(err)
			response.RespError(ctx, code.NotFound)
			return
		} else if err != nil {
			logrus.Error(err)
			response.RespError(ctx, code.ServerErrDB)
			return
		}
		mo, err = db.DB.MissionOrder.Query().
			WithMission().
			Where(missionorder.MissionIDEQ(_mission.ID)).
			First(ctx)
		if cep_ent.IsNotFound(err) {
			logrus.Error(err)
			response.RespError(ctx, code.InvalidParams)
			return
		}
	} else {
		var num int
		num, err = strconv.Atoi(req.Info)
		missionID := int64(num)
		mo, err = db.DB.MissionOrder.Query().Where(missionorder.MissionIDEQ(missionID)).WithMission().First(ctx)
		if cep_ent.IsNotFound(err) {
			logrus.Error(err)
			response.RespError(ctx, code.InvalidParams)
			return
		}
	}
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	userInfo, err := db.DB.User.Query().
		Where(user.IDEQ(mo.ConsumeUserID)).
		WithWallets(func(query *cep_ent.WalletQuery) {
			query.WithSymbol()
		}).First(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	count, err := db.DB.RenewalAgreement.Query().Where(renewalagreement.MissionIDEQ(mo.MissionID)).Count(ctx)
	if err != nil {
		logrus.Error(err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	set := types.InitBillingTypeToFrontMissionMap()
	missionBillingTypes := set.Values(mo.MissionBillingType)
	var resp MissionInfoResp
	if count >= 1 {
		resp.IsRenew = true
	} else {
		resp.IsRenew = false
	}
	_mission, err := db.DB.Mission.Query().Where(mission.IDEQ(mo.MissionID)).First(ctx)
	if err != nil {
		logrus.Errorf("query misison failed, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	resp.BillingMode = missionBillingTypes
	resp.MissionOrder = mo
	resp.Phone = userInfo.Phone
	resp.Amount = userInfo.Edges.Wallets
	resp.Name = _mission.Username
	resp.Password = _mission.Password
	response.RespSuccess(ctx, resp)
}

type MissionInfoResp struct {
	*cep_ent.MissionOrder
	Phone       string            `json:"phone"`
	Amount      []*cep_ent.Wallet `json:"amount"`
	IsRenew     bool              `json:"is_renew"`
	BillingMode string            `json:"billing_mode"`
	Name        string            `json:"name"`
	Password    string            `json:"password"`
}

type AddArtworkReq struct {
	Url   string `form:"url" json:"url" binding:"required"`
	Phone string `form:"phone" json:"phone"`
	Name  string `form:"name" json:"name" binding:"required"`
}

// AddArtwork 添加用户作品
func AddArtwork(ctx *gin.Context) {
	var req AddArtworkReq
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 根据手机号获取作者 ID
	var userID int64
	_user, err := db.DB.User.Query().Where(user.PhoneEQ(req.Phone)).First(ctx)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			userID = 0
		} else {
			logrus.Errorf("query user by phone failed, err:%v", err)
			response.RespError(ctx, code.ServerErrDB)
			return
		}
	}
	if _user != nil {
		userID = _user.ID
	}

	artwork, err := db.DB.Artwork.Create().SetAuthorID(userID).SetName(req.Name).SetURL(req.Url).Save(ctx)
	if err != nil {
		logrus.Errorf("create artwork failed, err:%v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	response.RespSuccess(ctx, artwork)
}

type CloseMissionReq struct {
	ID int64 `json:"id,string"`
}

// CloseMission 关闭任务
func CloseMission(c *gin.Context) {
	var req CloseMissionReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	var _mission *cep_ent.Mission

	txErr := db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		exist, err := tx.Mission.Query().Where(mission.DeletedAtEQ(common.ZeroTime), mission.IDEQ(req.ID)).Exist(c)
		if err != nil {
			logrus.Errorf("query mission(%d) failed, err: %v", req.ID, err)
			response.RespError(c, code.ServerErrDB)
			return err
		}
		if !exist {
			err = errors.New("任务 ID 不存在")
			logrus.Warningf("mission(%d) not exist, err: %v", req.ID, err)
			response.RespErrorWithMsg(c, code.NotFound, err.Error())
			return err
		}

		_mission, err = tx.Mission.UpdateOneID(req.ID).
			Where(mission.StateIn(enums.MissionStateSupplying, enums.MissionStateWaiting, enums.MissionStateDoing)).
			SetState(enums.MissionStateClosing).
			SetStatus(enums.MissionStatusClosing).
			SetCloseWay(enums.CloseWayAdmin).
			SetClosedAt(time.Now()).Save(c)
		if cep_ent.IsNotFound(err) {
			logrus.Errorf("closing mission(id: %d) get db not found, err: %v", req.ID, err)
			response.RespErrorWithMsg(c, code.InvalidParams, errors.New("任务处在不可关闭状态").Error())
			return err
		} else if err != nil {
			logrus.Errorf("failed to close mission(id: %d), err: %v", req.ID, err)
			response.RespError(c, code.ServerErrDB)
			return err
		}

		return nil
	})
	if txErr != nil {
		return
	}

	response.RespSuccess(c, _mission)
}

// ListGames 查询活动列表
func ListGames(ctx *gin.Context) {

	games, err := db.DB.Lotto.Query().Where(lotto.DeletedAtEQ(common.ZeroTime)).All(ctx)
	if err != nil {
		logrus.Errorf("failed to query games, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	response.RespSuccess(ctx, games)
}

// AddGame 添加活动
func AddGame(ctx *gin.Context) {
	var req types.AddGameReq
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 检查是否存在同名的活动
	exist, err := db.DB.Lotto.Query().Where(lotto.NameEQ(req.Name)).Exist(ctx)
	if err != nil {
		logrus.Errorf("failed to query lotto, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	if exist {
		response.RespErrorWithMsg(ctx, code.InvalidParams, "非法的活动名称")
		return
	}
	var startedAt *time.Time
	var endedAt *time.Time

	if req.StartedAt != nil {
		startedAt, err = _common.StringPtr2TimePtr(*req.StartedAt)
		if err != nil {
			logrus.Errorf("failed to transform timestamp")
			response.RespError(ctx, code.ServerErr)
			return
		}
	}
	if req.EndedAt != nil {
		endedAt, err = _common.StringPtr2TimePtr(*req.EndedAt)
		if err != nil {
			logrus.Errorf("failed to transform timestamp")
			response.RespError(ctx, code.ServerErr)
			return
		}
	}

	if req.StartedAt != nil && req.EndedAt != nil && startedAt.After(*endedAt) {
		err := errors.New("开始时间必须大于结束时间")
		logrus.Error(err)
		response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
		return
	}

	create := db.DB.Lotto.Create().
		SetName(req.Name).
		SetTotalWeight(req.TotalWeight).
		SetStatus(enums.LottoStatusNormal)

	if startedAt != nil && endedAt != nil {
		create.SetStartedAt(*startedAt).
			SetEndedAt(*endedAt)
	}

	game, err := create.Save(ctx)
	if err != nil {
		logrus.Errorf("failed to create game, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	response.RespSuccess(ctx, game)
}

// AddPrizes 添加奖品
func AddPrizes(ctx *gin.Context) {
	var req types.AddPrizesReq
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	var prizes []*cep_ent.LottoPrize

	for _, prize := range req.Prizes {
		if prize.PriceType == lottoprize.TypeGetCep && prize.CepAmount == 0 {
			err := errors.New("获得 Cep 奖品类型必须有 Cep 金额")
			logrus.Error(err)
			response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
			return
		}
	}

	txErr := db_utils.WithTx(ctx, nil, func(tx *cep_ent.Tx) error {
		game, err := tx.Lotto.Query().Where(lotto.ID(req.GameID)).
			WithLottoPrizes(func(prizeQuery *cep_ent.LottoPrizeQuery) {
				prizeQuery.Where(lottoprize.DeletedAtEQ(common.ZeroTime))
			}).First(ctx)
		if err != nil && !cep_ent.IsNotFound(err) {
			logrus.Errorf("failed to query prizes, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}

		// 计算已有奖品总权重
		var totalWeight int64
		for _, prize := range game.Edges.LottoPrizes {
			totalWeight += prize.Weight
		}

		// 加上要添加奖品权重
		for _, prize := range req.Prizes {
			totalWeight += prize.Weight
		}

		// 判断权重是否合理
		if game.TotalWeight < totalWeight {
			err = errors.New("奖品权重总和大于活动总权重,添加奖品失败")
			logrus.Error("奖品权重总和大于活动总权重,添加奖品失败")
			response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
			return err
		}

		// 批量创建奖品
		var prizesCreates []*cep_ent.LottoPrizeCreate
		for _, prize := range req.Prizes {
			create := tx.LottoPrize.Create().
				SetName(prize.Name).
				SetLevelName(prize.LevelName).
				SetLottoID(req.GameID).
				SetStatus(PrizeStatusNormal).
				SetWeight(prize.Weight).
				SetType(prize.PriceType)

			if prize.PriceType == lottoprize.TypeGetCep {
				create.SetCepAmount(prize.CepAmount)
			}

			prizesCreates = append(prizesCreates, create)
		}

		prizes, err = tx.LottoPrize.CreateBulk(prizesCreates...).Save(ctx)
		if err != nil {
			logrus.Errorf("failed to save przies, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}
		return nil
	})
	if txErr != nil {
		return
	}

	response.RespSuccess(ctx, prizes)
}

// GetOneGame 获取活动
func GetOneGame(ctx *gin.Context) {
	var req types.GetOneGameReq
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	game, err := db.DB.Lotto.Query().
		Where(lotto.IDEQ(req.ID), lotto.DeletedAtEQ(common.ZeroTime)).
		WithLottoPrizes(func(prizeQuery *cep_ent.LottoPrizeQuery) {
			prizeQuery.Where(lottoprize.DeletedAtEQ(common.ZeroTime)).
				Order(cep_ent.Asc(lottoprize.FieldWeight))
		}).
		First(ctx)
	if err != nil {
		logrus.Errorf("failed to get one game(id: %d), err: %v", req.ID, err)
		response.RespError(ctx, code.NotFound)
		return
	} else if err != nil {
		logrus.Errorf("failed to get one game(id: %d), err: %v", req.ID, err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	response.RespSuccess(ctx, game)
}

// DeleteOneGame 删除活动
func DeleteOneGame(ctx *gin.Context) {
	var req types.DeleteOneGameReq
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	txErr := db_utils.WithTx(ctx, nil, func(tx *cep_ent.Tx) error {
		// 软删除活动
		err := tx.Lotto.UpdateOneID(req.ID).
			SetDeletedAt(time.Now()).
			Exec(ctx)
		if err != nil {
			logrus.Errorf("failed to update game(id: %d), err: %v", req.ID, err)
			return err
		}

		// 软删除相关奖品
		err = tx.LottoPrize.Update().Where(lottoprize.LottoIDEQ(req.ID)).SetDeletedAt(time.Now()).Exec(ctx)
		if err != nil {
			logrus.Errorf("failed to update game(id: %d) prizes, err: %v", req.ID, err)
			return err
		}

		return nil

	})
	if txErr != nil {
		response.RespError(ctx, code.ServerErrDB)
	}

	games, err := db.DB.Lotto.Query().Where(lotto.DeletedAtEQ(common.ZeroTime)).All(ctx)
	if err != nil {
		logrus.Errorf("failed to query games, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	response.RespSuccess(ctx, games)
}

// DeletePrizes 删除奖品
func DeletePrizes(ctx *gin.Context) {
	var req types.DeletePrizesReq
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	err := db.DB.LottoPrize.Update().Where(lottoprize.LottoIDEQ(req.GameID), lottoprize.IDIn(req.PrizesID...)).SetDeletedAt(time.Now()).Exec(ctx)
	if err != nil {
		logrus.Errorf("failed to delete prizes, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	game, err := db.DB.Lotto.Query().
		Where(lotto.IDEQ(req.GameID), lotto.DeletedAtEQ(common.ZeroTime)).
		WithLottoPrizes(func(prizeQuery *cep_ent.LottoPrizeQuery) {
			prizeQuery.Where(lottoprize.DeletedAtEQ(common.ZeroTime)).
				Order(cep_ent.Asc(lottoprize.FieldWeight))
		}).
		First(ctx)
	if err != nil {
		logrus.Errorf("failed to get one game(id: %d), err: %v", req.GameID, err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	response.RespSuccess(ctx, game)
}

// UpdateOneGame 更新活动
func UpdateOneGame(ctx *gin.Context) {
	var req types.UpdateOneGameReq
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	var newGame *cep_ent.Lotto

	txErr := db_utils.WithTx(ctx, nil, func(tx *cep_ent.Tx) error {
		game, err := tx.Lotto.Query().
			Where(lotto.IDEQ(req.ID), lotto.DeletedAtEQ(common.ZeroTime)).
			WithLottoPrizes(func(prizeQuery *cep_ent.LottoPrizeQuery) {
				prizeQuery.Where(lottoprize.DeletedAtEQ(common.ZeroTime))
			}).
			First(ctx)
		if err != nil {
			logrus.Errorf("failed to get one game(id: %d), err: %v", req.ID, err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}

		// 检查是否存在同名的活动
		exist, err := db.DB.Lotto.Query().Where(lotto.NameEQ(req.Name)).Exist(ctx)
		if err != nil {
			logrus.Errorf("failed to query lotto, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}

		if exist {
			err = errors.New("非法的活动名称")
			response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
			return err
		}

		// 计算已有奖品总权重
		var totalWeight int64
		for _, prize := range game.Edges.LottoPrizes {
			totalWeight += prize.Weight
		}

		// 校验权重
		if req.TotalWeight > 0 && totalWeight > req.TotalWeight {
			err = errors.New("奖品权重总和大于活动总权重,修改权重失败")
			logrus.Error("奖品权重总和大于活动总权重,修改权重失败")
			response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
			return err
		}

		var startedAt *time.Time
		var endedAt *time.Time

		if req.StartedAt != nil {
			startedAt, err = _common.StringPtr2TimePtr(*req.StartedAt)
			if err != nil {
				logrus.Errorf("failed to transform timestamp")
				response.RespError(ctx, code.ServerErr)
				return err
			}
		}
		if req.EndedAt != nil {
			endedAt, err = _common.StringPtr2TimePtr(*req.EndedAt)
			if err != nil {
				logrus.Errorf("failed to transform timestamp")
				response.RespError(ctx, code.ServerErr)
				return err
			}
		}

		// 传入开始时间
		if req.StartedAt != nil {
			// 传入结束时间 传入开始时间不能晚于传入结束时间
			if req.EndedAt != nil && startedAt.After(*endedAt) {
				err = errors.New("开始时间必须早于结束时间")
				logrus.Error("开始时间必须早于结束时间,修改权重失败")
				response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
				return err
			} else if req.EndedAt == nil && !game.EndedAt.Equal(common.ZeroTime) && startedAt.After(game.EndedAt) {
				// 没有传入结束时间 并且已有有结束时间 传入开始时间不能晚于原本结束时间
				err = errors.New("开始时间必须早于结束时间")
				logrus.Error("开始时间必须早于结束时间,修改权重失败")
				response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
				return err
			}
		}

		// 只传入结束时间 传入结束时间不能早于原本开始时间
		if req.EndedAt != nil && req.StartedAt == nil && !game.StartedAt.Equal(common.ZeroTime) && endedAt.Before(game.StartedAt) {
			err = errors.New("开始时间必须早于结束时间")
			logrus.Error("开始时间必须早于结束时间,修改权重失败")
			response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
		}

		update := tx.Lotto.UpdateOne(game)
		if len(req.Name) > 0 {
			update.SetName(req.Name)
		}
		if req.StartedAt != nil {
			update.SetStartedAt(*startedAt)
		}
		if req.EndedAt != nil {
			update.SetEndedAt(*endedAt)
		}
		if req.TotalWeight > 0 {
			update.SetTotalWeight(req.TotalWeight)
		}
		newGame, err = update.Save(ctx)
		if err != nil {
			logrus.Error("failed to update game(ID: %d), err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return err
		}

		return nil
	})
	if txErr != nil {
		return
	}

	response.RespSuccess(ctx, newGame)
}

func UpdatePrizes(ctx *gin.Context) {
	var req types.UpdatePrizesReq
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	for _, prize := range req.Prizes {
		if prize.PriceType == lottoprize.TypeGetCep && prize.CepAmount == 0 {
			err := errors.New("获得 Cep 奖品类型必须有 Cep 金额")
			logrus.Error(err)
			response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
			return
		}
	}

	// 统计已有奖品且不需要修改产品 ID
	prizeIDs := make([]int64, len(req.Prizes))
	for idx, prize := range req.Prizes {
		if prize.Weight > 0 {
			prizeIDs[idx] = prize.PrizeID
		}
	}

	var newGame *cep_ent.Lotto

	txErr := db_utils.WithTx(ctx, nil, func(tx *cep_ent.Tx) error {
		game, err := tx.Lotto.Query().
			Where(lotto.IDEQ(req.GameID), lotto.DeletedAtEQ(common.ZeroTime)).
			WithLottoPrizes(func(prizeQuery *cep_ent.LottoPrizeQuery) {
				prizeQuery.Where(lottoprize.DeletedAt(common.ZeroTime), lottoprize.LottoIDNotIn(prizeIDs...))
			}).First(ctx)
		if err != nil {
			return err
		}

		// 计算已有奖品总权重
		var totalWeight int64
		for _, prize := range game.Edges.LottoPrizes {
			totalWeight += prize.Weight
		}

		// 加上要添加奖品权重
		for _, prize := range req.Prizes {
			totalWeight += prize.Weight
		}

		// 判断权重是否合理
		if game.TotalWeight < totalWeight {
			err = errors.New("奖品权重总和大于活动总权重,添加奖品失败")
			logrus.Error("奖品权重总和大于活动总权重,添加奖品失败")
			response.RespErrorWithMsg(ctx, code.InvalidParams, err.Error())
			return err
		}

		// 更新奖品
		for _, prize := range req.Prizes {
			update := tx.LottoPrize.Update().
				Where(lottoprize.LottoIDEQ(req.GameID),
					lottoprize.IDEQ(prize.PrizeID),
					lottoprize.DeletedAt(common.ZeroTime)).
				SetType(prize.PriceType)

			if len(prize.LevelName) > 0 {
				update.SetLevelName(prize.LevelName)
			}
			if prize.Weight > 0 {
				update.SetWeight(prize.Weight)
			}
			if len(prize.Name) > 0 {
				update.SetName(prize.Name)
			}
			if len(prize.Status) > 0 {
				update.SetStatus(prize.Status)
			}
			if prize.PriceType == lottoprize.TypeGetCep && prize.CepAmount > 0 {
				update.SetCepAmount(prize.CepAmount)
			}

			err = update.Exec(ctx)
			if err != nil {
				logrus.Errorf("failed to update prizes, err: %v", err)
				response.RespError(ctx, code.ServerErrDB)
				return err
			}
		}

		newGame, err = tx.Lotto.Query().
			Where(lotto.IDEQ(req.GameID), lotto.DeletedAtEQ(common.ZeroTime)).
			WithLottoPrizes(func(prizeQuery *cep_ent.LottoPrizeQuery) {
				prizeQuery.Where(lottoprize.DeletedAt(common.ZeroTime), lottoprize.LottoIDNotIn(prizeIDs...))
			}).First(ctx)
		if err != nil {
			return err
		}

		return nil
	})
	if txErr != nil {
		return
	}

	response.RespSuccess(ctx, newGame)
}

// ListGameChanceRules 查询用户获得抽奖次数规则列表
func ListGameChanceRules(c *gin.Context) {
	var req types.ListGameChanceRulesReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	game, err := db.DB.Lotto.Query().
		Where(lotto.DeletedAtEQ(common.ZeroTime), lotto.IDEQ(req.GameID)).
		WithLottoChangeRules(func(ruleQuery *cep_ent.LottoChanceRuleQuery) {
			ruleQuery.Where(lottochancerule.DeletedAtEQ(common.ZeroTime)).
				Order(cep_ent.Asc(lottochancerule.FieldAwardCount))
		}).First(c)
	if err != nil {
		logrus.Errorf("query game(id: %d) condition failed, err: %v", req.GameID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, game)
}

// AddGameChanceRules 添加规则
func AddGameChanceRules(c *gin.Context) {
	var req types.AddGameChanceRulesReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 参数校验 充值类型必须要有充值金额
	for _, rule := range req.Rules {
		if rule.Condition == enums.LottoConditionRecharge && rule.Amount == 0 {
			logrus.Errorf("recharge rules recharge amount is missing")
			response.RespErrorWithMsg(c, code.InvalidParams, "充值类型必须有充值金额")
			return
		}
	}

	var ruleCreate []*cep_ent.LottoChanceRuleCreate
	for _, rule := range req.Rules {
		create := db.DB.LottoChanceRule.Create().
			SetLottoID(req.GameID).
			SetAwardCount(rule.AwardCount).
			SetCondition(rule.Condition).
			SetRechargeAmount(rule.Amount)
		ruleCreate = append(ruleCreate, create)
	}

	rules, err := db.DB.LottoChanceRule.CreateBulk(ruleCreate...).Save(c)
	if err != nil {
		logrus.Errorf("create lotto(id: %d) chance rule failed, err: %v", req.GameID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, rules)
}

// UpdateGameChanceRules 修改规则
func UpdateGameChanceRules(c *gin.Context) {
	var req types.UpdateGameChanceRulesReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 参数校验 充值类型必须要有充值金额
	for _, rule := range req.Rules {
		if rule.Condition == enums.LottoConditionRecharge && rule.Amount == 0 {
			logrus.Errorf("recharge rules recharge amount is missing")
			response.RespErrorWithMsg(c, code.InvalidParams, "充值类型必须有充值金额")
			return
		}
	}

	for _, rule := range req.Rules {
		err := db.DB.LottoChanceRule.Update().
			Where(lottochancerule.LottoIDEQ(req.GameID), lottochancerule.IDEQ(rule.RuleID)).
			SetAwardCount(rule.AwardCount).
			SetCondition(rule.Condition).
			SetRechargeAmount(rule.Amount).Exec(c)
		if err != nil {
			logrus.Errorf("update lotto rule(id: %d) failed, err: %v", rule.RuleID, err)
			response.RespError(c, code.ServerErrDB)
			return
		}
	}

	rules, err := db.DB.LottoChanceRule.Query().
		Where(lottochancerule.DeletedAtEQ(common.ZeroTime)).
		Order(cep_ent.Asc(lottochancerule.FieldAwardCount)).
		All(c)
	if err != nil {
		logrus.Errorf("query lotto chance rule failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, rules)
}

// DeleteGameChanceRules 删除规则
func DeleteGameChanceRules(c *gin.Context) {
	var req types.DeleteGameChanceRulesReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	err := db.DB.LottoChanceRule.Update().
		Where(lottochancerule.LottoIDEQ(req.GameID), lottochancerule.IDIn(req.RulesIDs...)).
		SetDeletedAt(time.Now()).
		Exec(c)
	if err != nil {
		logrus.Errorf("failed to delete lotto chance rule, err: %v", err)
		return
	}

	rules, err := db.DB.LottoChanceRule.Query().
		Where(lottochancerule.DeletedAtEQ(common.ZeroTime)).
		Order(cep_ent.Asc(lottochancerule.FieldAwardCount)).
		All(c)
	if err != nil {
		logrus.Errorf("query lotto chance rule failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, rules)
}

func ListPrizeWheelWhiteList(c *gin.Context) {
	var (
		notLimitUsers = make([]string, 0)
		err           error
	)

	notLimitUsers, err = myredis.Client.LRange(c, types.RedisKeyLottoGameNotLimitUser, 0, -1).Result()
	if err != nil {
		logrus.Errorf("redis lrange not limit user failed, err: %v", err)
		response.RespError(c, code.ServerErrCache)
		return
	}

	userIDs := make([]int64, len(notLimitUsers))
	for idx, limitUserID := range notLimitUsers {
		userID, err := strconv.ParseInt(limitUserID, 10, 64)
		if err != nil {
			logrus.Errorf("failed to parse int, err: %v", err)
			return
		}
		userIDs[idx] = userID
	}

	users, err := db.DB.User.Query().Where(user.DeletedAt(common.ZeroTime), user.IDIn(userIDs...)).All(c)
	if cep_ent.IsNotFound(err) {
		response.RespError(c, code.NotFound)
		return
	} else if err != nil {
		logrus.Errorf("failed to query Prize Wheel White List, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	phoneList := make([]string, len(notLimitUsers))
	for idx, _user := range users {
		phoneList[idx] = _user.Phone
	}

	response.RespSuccess(c, phoneList)
}

func AddPrizeWheelWhiteListUser(c *gin.Context) {
	phone := c.Param("phone")
	if phone == "" {
		response.RespErrorInvalidParams(c, errors.New("phone is empty"))
		return
	}

	// 根据手机号查询到用户 ID
	_user, err := db.DB.User.Query().Where(user.PhoneEQ(phone)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		}
		response.RespError(c, code.ServerErrDB)
		return
	}

	if err = myredis.Client.LPush(c, types.RedisKeyLottoGameNotLimitUser, _user.ID).Err(); err != nil {
		response.RespError(c, code.ServerErrCache)
		return
	}

	response.RespSuccess(c, _user)
}

func DeletePrizeWheelWhiteListUser(c *gin.Context) {
	phone := c.Param("phone")
	if phone == "" {
		response.RespErrorInvalidParams(c, errors.New("phone is empty"))
		return
	}

	// 根据手机号查询到用户 ID
	_user, err := db.DB.User.Query().Where(user.PhoneEQ(phone)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
			return
		}
		response.RespError(c, code.ServerErrDB)
		return
	}

	if err = myredis.Client.LRem(c, types.RedisKeyLottoGameNotLimitUser, 0, _user.ID).Err(); err != nil {
		response.RespError(c, code.ServerErrCache)
		return
	}

	response.RespSuccess(c, _user)
}
