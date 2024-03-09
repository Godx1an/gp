package controllers

import (
	"crypto/md5"
	"entgo.io/ent/dialect/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/bill"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/costaccount"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/device"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/invite"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/mission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/missionconsumeorder"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/profitsetting"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/wallet"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/h_mac"
	"github.com/stark-sim/serializer/response"
	"strconv"
	"time"
	"vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/logic"

	"vpay/internal/handlers"
	"vpay/internal/types"
	"vpay/utils"
)

func getUserId(c *gin.Context) (int64, error) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		response.RespError(c, code.AuthFailed)
		return 0, handlers.FAIL_OF_CONFIRM
	}
	return userID, nil
}

func GetDeviceUserByCondition(c *gin.Context) {
	var req types.ReqDeviceUserCondition
	err := c.Bind(&req)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	var userId int64
	if req.UserId != "" {
		userId, err = strconv.ParseInt(req.UserId, 10, 64)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErr, "服务端错误，用户 id 转译失败")
			return
		}
	}
	op := types.OptionDeviceUser{
		UserId:       userId,
		RegisterTime: req.RegisterTime,
		ProfitWay:    req.ProfitWay,
		IsFrozen:     req.IsFrozen,
		PaginateReq:  req.PaginateReq,
	}
	query := db.DB.User.Query().
		Where(user.HasDevicesWith(device.DeletedAt(utils.ZeroTime)), user.DeletedAtEQ(utils.ZeroTime)).
		WithProfitSettings().WithDevices(func(deviceQuery *cep_ent.DeviceQuery) {
		deviceQuery.Where(device.Linking(false))
	}).WithProfitSettings().WithProfitAccount()
	if op.RegisterTime != utils.ZeroTime {
		query = query.Where(user.CreatedAtGTE(op.RegisterTime))
	}
	if op.UserId != 0 {
		query = query.Where(user.ID(op.UserId))
	}
	if op.ProfitWay != 0 {
		query.Where(user.HasProfitSettingsWith(profitsetting.Ratio(op.ProfitWay)))
	}
	if op.IsFrozen != nil {
		query.Where(user.IsFrozen(*op.IsFrozen))
	}
	totalCount, err := query.Count(c)
	users, err := query.Limit(op.PageSize).Offset(op.PageSize * (op.PageIndex - 1)).All(c)
	if err != nil {
		logrus.Errorf("db query users by condition err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	listData := serializer.FrontendSerialize(users)
	for _, itemData := range listData.([]interface{}) {
		itemData.(map[string]interface{})["device_number"] = len(itemData.(map[string]interface{})["edges"].(map[string]interface{})["devices"].([]interface{}))
		delete(itemData.(map[string]interface{})["edges"].(map[string]interface{}), "devices")
	}

	resp := common_types.PaginateResp{
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
		Total:     totalCount,
		List:      listData,
	}
	response.RespSuccess(c, resp)
}

func GetCommonUserByCondition(c *gin.Context) {
	var req types.ReqUserCondition
	err := c.ShouldBind(&req)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	op := types.OptionUser{
		CreatedTime: req.CreatedTime,
		IsFrozen:    req.IsFrozen,
		PaginateReq: req.PaginateReq,
	}
	if req.UserId != "" {
		op.UserId, err = strconv.ParseInt(req.UserId, 10, 64)
		if err != nil {
			fmt.Println(err)
			response.RespError(c, code.InvalidParams)
			return
		}
	}
	query := db.DB.User.Query().WithCostAccount().
		Where(user.DeletedAt(utils.ZeroTime), user.HasCostAccountWith(costaccount.DeletedAt(utils.ZeroTime)))
	if op.UserId != 0 {
		query = query.Where(user.ID(op.UserId))
	}
	if op.CreatedTime != utils.ZeroTime {
		query = query.Where(user.CreatedAtGTE(op.CreatedTime))
	}
	if op.IsFrozen != nil {
		query = query.Where(user.IsFrozen(*op.IsFrozen))
	}
	totalCount, err := query.Count(c)
	users, err := query.Limit(op.PageSize).Offset(op.PageSize * (op.PageIndex - 1)).All(c)
	serializerData := serializer.FrontendSerialize(users)
	for _, itemData := range serializerData.([]interface{}) {
		userId, err := strconv.ParseInt(itemData.(map[string]interface{})["id"].(string), 10, 64)
		itemData.(map[string]interface{})["user_type"], err = handlers.JudgeYHOrEx(c, userId)
		if err != nil {
			logrus.Error(err)
			response.RespErrorWithMsg(c, code.ServerErr, "服务端错误")
			return
		}
	}
	resp := &common_types.PaginateResp{
		PageIndex: op.PageIndex,
		PageSize:  op.PageSize,
		Total:     totalCount,
		List:      serializerData,
	}
	response.RespSuccess(c, resp)
}

func GetKey(c *gin.Context) {
	userid, err := common.GetUserId(c)
	if err != nil {
		return
	}

	dbUser, err := db.DB.User.Query().Where(user.ID(userid)).First(c)
	if err != nil {
		logrus.Errorf("db query user err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	ans := types.KeyAndSecret{
		Key:    dbUser.Key,
		Secret: dbUser.Secret,
	}

	response.RespSuccess(c, serializer.FrontendSerialize(ans))
}

func HmacEncode(c *gin.Context) {
	var encodeInfo types.EncodeInfo
	if err := c.ShouldBind(&encodeInfo); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	userid, err := common.GetUserId(c)
	if err != nil {
		return
	}
	encodeInfo.UserId = userid
	ans := new(types.ResEncodeInfo)
	if ans, err = handlers.EncodeInfo(c, encodeInfo); err != nil {
		logrus.Errorf("encode info err: %v", err)
		response.RespErrorWithMsg(c, code.ServerErr, "加密失败")
		return
	}
	response.RespSuccess(c, ans)
}

// MD5ForHmacSecret 为外部提供 md5 加密，因为加密请求体接口需要检验用户拥有 secret 而不只是 key
func MD5ForHmacSecret(c *gin.Context) {
	type input struct {
		HmacSecret string `json:"hmac_secret"`
	}
	var req input
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	result := fmt.Sprintf("%x", md5.Sum([]byte(req.HmacSecret)))
	response.RespSuccess(c, result)
}

func HmacEncodeForReal(c *gin.Context) {
	// 获取请求内容部分
	body, err := c.GetRawData()
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	key := c.Request.Header.Get("Hmac-Key")
	if key == "" {
		response.RespErrorInvalidParams(c, errors.New("Hmac-Key 为空"))
		return
	}
	hashedSecret := c.Request.Header.Get("Hashed-Secret")
	if hashedSecret == "" {
		response.RespErrorInvalidParams(c, errors.New("Hashed-Secret 为空"))
		return
	}

	requestURI := c.Request.Header.Get("URI")
	if requestURI == "" {
		response.RespErrorInvalidParams(c, errors.New("URI 为空"))
		return
	}

	// 检验部分
	// 密钥键值要成对，通过 key 找到密钥对，然后 md5 加密 secret 看和参数一不一致
	var hmacSecret string
	var keyPairMission *cep_ent.Mission

	// 先找出对应的密钥对
	hmacKeyPair, err := db.DB.User.Query().Where(user.Key(key), user.DeletedAt(utils.ZeroTime)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			keyPairMission, err = db.DB.Mission.Query().
				Where(
					mission.TypeEQ(enums.MissionTypeKeyPair),
					mission.StatusEQ(enums.MissionStatusDoing),
					mission.DeletedAt(utils.ZeroTime),
					mission.TempHmacKey(key),
				).
				First(c)
			if err != nil {
				if cep_ent.IsNotFound(err) {
					response.RespErrorWithMsg(c, code.ServerErr, "加密失败")
				} else {
					response.RespErrorWithMsg(c, code.ServerErrDB, err.Error())
				}
				return
			}
		} else {
			response.RespErrorWithMsg(c, code.ServerErrDB, err.Error())
			return
		}
	}
	// 检查 secret
	if keyPairMission != nil {
		hmacSecret = keyPairMission.TempHmacSecret
	} else {
		hmacSecret = hmacKeyPair.Secret
	}
	currentHashedSecret := fmt.Sprintf("%x", md5.Sum([]byte(hmacSecret)))
	if currentHashedSecret != hashedSecret {
		response.RespErrorWithMsg(c, code.InvalidParams, "secret 不匹配")
		return
	}

	// 加密部分
	res, err := h_mac.GenerateHMAC(requestURI, hmacSecret, body)
	if err != nil {
		logrus.Errorf("encode info for client err: %v", err)
		response.RespErrorWithMsg(c, code.ServerErr, "加密失败")
		return
	}
	response.RespSuccess(c, res)
}

func HmacEncodeForClient(c *gin.Context) {

	body, err := c.GetRawData()
	key := c.Request.Header.Get("Hmac-key")
	requestURI := c.Request.Header.Get("URI")
	if err != nil {
		//c.AbortWithStatus(http.StatusBadRequest)
		response.RespErrorInvalidParams(c, err)
		return
	}
	if key == "" {
		response.RespErrorInvalidParams(c, errors.New("Hmac-Key为空"))
		return
	}
	if requestURI == "" {
		response.RespErrorInvalidParams(c, errors.New("URI为空"))
		return
	}

	var res string
	if res, err = handlers.EncodeInfoForClient(c, body, requestURI, key); err != nil {
		logrus.Errorf("encode info for client err: %v", err)
		response.RespErrorWithMsg(c, code.ServerErr, "加密失败")
		return
	}

	response.RespSuccess(c, res)
}

func AllCostUserDoingMission(c *gin.Context) {
	users, err := db.DB.User.Query().Where(user.DeletedAtEQ(utils.ZeroTime), user.HasMissionConsumeOrdersWith(missionconsumeorder.StatusEQ(enums.MissionOrderStatusSupplying))).WithCostAccount().All(c)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErrDB, "查询用户失败")
		return
	}

	var res = make([]types.AllCostUserDoingMissionResp, len(users))
	for idx, mUser := range users {
		res[idx].UserId = strconv.Itoa(int(mUser.ID))
		res[idx].TotalCep = mUser.Edges.CostAccount.TotalCep
		res[idx].Key = mUser.Key
		res[idx].Secret = mUser.Secret
	}

	response.RespSuccess(c, res)
}

func ListUserMissionConsumeOrder(c *gin.Context) {
	var req types.ListUserMissionConsumeOrderReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	userId, err := common.GetUserId(c)
	if err != nil {
		logrus.Errorf("get user id err: %v", err)
		response.RespError(c, code.UnLogin)
		return
	}

	// 查询用户的任务消费订单
	query := db.DB.MissionConsumeOrder.Query().Where(missionconsumeorder.UserIDEQ(userId), missionconsumeorder.DeletedAtEQ(utils.ZeroTime))
	if req.Type != "" {
		query = query.Where(missionconsumeorder.TypeEQ(enums.MissionType(req.Type)))
	}

	if len(req.Status) > 0 {
		// 处理多个状态
		var orderStatus []enums.MissionOrderStatus
		for _, status := range req.Status {
			orderStatus = append(orderStatus, enums.MissionOrderStatus(status))
		}
		query = query.Where(missionconsumeorder.StatusIn(orderStatus...))
	}

	total, err := query.Count(c)
	if err != nil {
		logrus.Errorf("user(%v) count mission consume order err: %v", userId, err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	consumeOrders, err := query.Limit(req.PageSize).Offset(req.PageSize * (req.PageIndex - 1)).All(c)
	if err != nil {
		logrus.Errorf("user(%v) list mission consume order err: %v", userId, err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	respData := common_types.PaginateResp{
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
		Total:     total,
		List:      consumeOrders,
	}

	response.RespSuccess(c, serializer.FrontendSerialize(respData))
}

// ListUserInviteCode 查询用户的邀请码列表
func ListUserInviteCode(c *gin.Context) {
	var req types.ListUserInviteCodeReq
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

	q := db.DB.Invite.Query().Where(invite.DeletedAtEQ(utils.ZeroTime)).Where(invite.UserIDEQ(userId))
	if req.InviteCode != "" {
		q = q.Where(invite.InviteCodeEQ(req.InviteCode))
	}
	if req.Type != "" {
		q = q.Where(invite.TypeEQ(req.Type))
	}

	invites, err := q.All(c)
	if err != nil {
		logrus.Errorf("db query invite err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, invites)
}

// CreateUserInviteCode 创建用户邀请码
func CreateUserInviteCode(c *gin.Context) {
	var req types.CreateUserInviteCodeReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	// fixme: 目前邀请赠送的全部由自己控制，所以先写死
	req.ShareCep = 1000
	req.RegCep = 1000
	var firstRechargeGiftCep int64 = 1000

	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}

	// 检查用户是否已经有此类型的邀请码了
	oldInviteInfo, err := db.DB.Invite.Query().Where(invite.DeletedAtEQ(utils.ZeroTime), invite.UserIDEQ(userId), invite.TypeEQ(req.Type)).First(c)
	if cep_ent.IsNotFound(err) {
		// 未找到，继续往下走
	} else if err != nil {
		logrus.Errorf("db query invite err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	} else {
		response.RespSuccess(c, oldInviteInfo)
		return
	}

	// 生成一个邀请码
	var inviteCode string
	for i := 0; i < 3; i++ {
		// 最多生成三次邀请码
		generateCode := utils.GetInvCodeByUIDUniqueNew(uint64(time.Now().UnixNano()), types.UserInviteCodeLength)
		// 检查邀请码是否已经存在
		exist, err := db.DB.Invite.Query().Where(invite.DeletedAtNEQ(utils.ZeroTime), invite.InviteCodeEQ(inviteCode)).Exist(c)
		if err != nil {
			logrus.Errorf("db query invite err: %v", err)
			response.RespError(c, code.ServerErrDB)
			return
		}
		if !exist {
			inviteCode = generateCode
			break
		}
	}

	if inviteCode == "" {
		response.RespErrorWithMsg(c, code.NotEnough, "生成邀请码失败（邀请码数量不足）")
		return
	}

	userInviteInfo, err := db.DB.Invite.Create().
		SetInviteCode(inviteCode).
		SetType(req.Type).
		SetShareCep(req.ShareCep).
		SetRegCep(req.RegCep).
		SetFirstRechargeCep(firstRechargeGiftCep).
		SetUserID(userId).
		Save(c)
	if err != nil {
		logrus.Errorf("db create invite err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, userInviteInfo)
}

// UpdateUserInviteCode 更新用户邀请码
func UpdateUserInviteCode(c *gin.Context) {
	var req types.UpdateUserInviteCodeReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	newInviteInfo, err := db.DB.Invite.Update().Where(invite.IDEQ(req.ID)).SetShareCep(req.ShareCep).SetRegCep(req.RegCep).Save(c)
	if err != nil {
		logrus.Errorf("db update invite err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, newInviteInfo)
}

// ListUserInviteUser 查询用户邀请的用户列表
func ListUserInviteUser(c *gin.Context) {
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

	query := db.DB.User.Query().Where(user.DeletedAtEQ(utils.ZeroTime), user.ParentIDEQ(userId))

	total, err := query.Count(c)
	if err != nil {
		logrus.Errorf("db count user err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	users, err := query.Offset(req.PageSize * (req.PageIndex - 1)).Limit(req.PageSize).All(c)
	if err != nil {
		logrus.Errorf("db query user err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	var resp common_types.PaginateResp
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = total
	resp.List = users

	response.RespSuccess(c, resp)
}

// ListUserInviteGift 查询用户邀请的奖励列表
func ListUserInviteGift(c *gin.Context) {
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

	query := db.DB.Bill.Query().Where(bill.DeletedAt(utils.ZeroTime), bill.TargetUserID(userId), bill.WayIn(enums.BillWayActiveShare, enums.BillWayFirstInviteRecharge, enums.BillWayActiveInviteRecharge, enums.BillWayActiveBind))

	total, err := query.Count(c)
	if err != nil {
		logrus.Errorf("db count cost bill err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	shareCostBills, err := query.Offset(req.PageSize * (req.PageIndex - 1)).Limit(req.PageSize).All(c)
	if err != nil {
		logrus.Errorf("db query cost bill err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	var resp common_types.PaginateResp
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = total
	resp.List = shareCostBills

	response.RespSuccess(c, resp)
}

// StatisticsUserInviteGift 统计用户邀请获得的奖励
func StatisticsUserInviteGift(c *gin.Context) {
	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}

	// 查询邀请活动总共赠送的 CEP
	var resp types.StatisticsUserInviteGiftResp
	resp.TotalInviteGiftCep, err = db.DB.Bill.Query().Where(bill.DeletedAt(utils.ZeroTime), bill.TargetUserID(userId)).Where(bill.WayIn(enums.BillWayActiveShare, enums.BillWayFirstInviteRecharge, enums.BillWayActiveInviteRecharge, enums.BillWayActiveBind), bill.SymbolID(types.CepSymbolID)).Aggregate(
		func(s *sql.Selector) string {
			return "COALESCE(SUM(amount), 0)"
		}).Int(c)
	if cep_ent.IsNotFound(err) {
		resp.TotalInviteGiftCep = 0
	} else if err != nil {
		logrus.Errorf("db sum cost bill err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, resp)
}

// GetUserPopVersion 查询用户pop_version字段信息
func GetUserPopVersion(c *gin.Context) {
	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}

	query := db.DB.User.Query().Where(user.IDEQ(userId))
	_user, err := query.First(c)
	if err != nil {
		logrus.Errorf("db query user err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	resp := struct {
		PopVersion string `json:"pop_version"`
	}{
		PopVersion: _user.PopVersion,
	}
	response.RespSuccess(c, serializer.FrontendSerialize(resp))
}

// UpdatePopVersion 更新用户pop_version字段信息
func UpdatePopVersion(c *gin.Context) {
	var req types.UpdatePopVersionReq
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

	query := db.DB.User.Query().Where(user.IDEQ(userId))
	_user, err := query.First(c)

	_, err = db.DB.User.Update().Where(user.IDEQ(userId)).SetPopVersion(req.PopVersion).Save(c)
	if err != nil {
		logrus.Errorf("db update invite err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, serializer.FrontendSerialize(_user))
}

// ListGpuInfo 获取用户绑定的GPU信息
func ListGpuInfo(c *gin.Context) {
	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}

	_device, err := db.DB.Device.Query().
		WithDeviceGpuMissions(func(query *cep_ent.DeviceGpuMissionQuery) {
			query.WithGpu()
		}).
		Where(device.UserIDEQ(userId)).
		All(c)
	if err != nil {
		logrus.Errorf("db update invite err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, _device)
}

// GetUserBalance 获取用户余额 fixme：待优化
func GetUserBalance(c *gin.Context) {
	userId, err := common.GetUserId(c)
	if err != nil {
		response.RespError(c, code.UnLogin)
		return
	}

	_wallet, err := db.DB.Wallet.Query().Where(wallet.UserID(userId), wallet.SymbolID(types.CepSymbolID)).First(c)
	if err != nil {
		logrus.Errorf("db get user wallet failed, err: %v", err)
		response.RespErrorWithMsg(c, code.ServerErrDB, err.Error())
		return
	}
	freezeCep, _ := logic.GetFrozenAmountByUserIDAndSymbolID(c, userId, types.CepSymbolID)
	realAmount := _wallet.Amount - freezeCep

	response.RespSuccess(c, realAmount)
}
