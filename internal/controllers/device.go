package controllers

import (
	"fmt"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/device"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/devicegpumission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/gpu"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/price"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"strconv"
	"strings"
	"time"
	"vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/handlers"
	"vpay/internal/logic"
	"vpay/internal/myredis"
	"vpay/internal/types"
	"vpay/pkg/jwt"
	"vpay/utils"
	db2 "vpay/utils/db_utils"
)

type DeviceHeartBeatReq struct {
	DeviceID int64         `uri:"id"`
	Status   device.Status `json:"status"`
}

func DeviceHeartBeat(c *gin.Context) {
	var req DeviceHeartBeatReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	_device, err := db.DB.Device.Query().Where(device.DeletedAt(utils.ZeroTime), device.ID(req.DeviceID)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespError(c, code.NotFound)
		} else {
			response.RespErrorWithMsg(c, code.ServerErr, err.Error())
		}
		return
	}
	// 更新心跳 5 分钟
	if err = myredis.Client.Set(c, fmt.Sprintf(types.RedisDeviceHeartBeat, req.DeviceID), string(req.Status), time.Minute*5).Err(); err != nil {
		response.RespError(c, code.ServerErrCache)
		return
	}
	response.RespSuccess(c, serializer.FrontendSerialize(_device))
}

func DeviceHeartBeat2(c *gin.Context) {
	var req types.DeviceHeartBeatReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	// 根据设备 ID 查询设备
	_device, err := db.DB.Device.Query().
		Where(device.ID(req.DeviceID), device.DeletedAt(utils.ZeroTime)).
		WithDeviceGpuMissions().
		First(c)
	if cep_ent.IsNotFound(err) {
		response.RespError(c, code.NotFound)
		return
	} else if err != nil {
		logrus.Errorf("query device %d err: %v", req.DeviceID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	logrus.Infof("device: %+v", _device)

	deviceStatus, _code, errMsg := handlers.DeviceHeartBeat(c, &handlers.DeviceHeartBeatOptions{
		Tx:       nil,
		DeviceID: &req.DeviceID,
		Device:   _device,
		GPUs:     req.GPUs,
	})
	if _code != code.Success {
		if _code == 40020 {
			response.RespErrorWithMsg(c, _code, errMsg.Error())
		} else {
			response.RespError(c, _code)
		}
	}

	deviceInfo, err := db.DB.Device.Query().
		Where(device.ID(req.DeviceID)).
		WithDeviceGpuMissions().
		First(c)
	if err != nil {
		logrus.Errorf("device info query occure error: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	// 更新心跳 5 分钟
	if err = myredis.Client.Set(c, fmt.Sprintf(types.RedisDeviceHeartBeat, req.DeviceID), string(deviceStatus), time.Minute*5).Err(); err != nil {
		response.RespError(c, code.ServerErrCache)
		return
	}
	response.RespSuccess(c, deviceInfo)
}

func BindDevice(c *gin.Context) {
	// 登录态获取 user_id
	userID, err := common.GetUserId(c)
	if err != nil {
		return
	}
	// 参数校验
	var req types.BindDeviceReq
	if err = c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	deviceID, err := strconv.ParseInt(req.DeviceID, 10, 64)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	_device, err := handlers.BindDevice(c, &handlers.BindDeviceOptions{
		Tx:       nil,
		UserID:   userID,
		DeviceID: deviceID,
	})
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	response.RespSuccess(c, serializer.FrontendSerialize(_device))
}

func GetDeviceByCondition(c *gin.Context) {
	// 参数校验
	var req types.ReqDevices
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	query := db.DB.Device.Query().Where(device.DeletedAt(utils.ZeroTime)).Limit(req.PageSize).Offset((req.PageIndex - 1) * req.PageSize)
	if req.DeviceID != nil {
		query = query.Where(device.SerialNumber(*req.DeviceID))
	}
	if req.UserId != nil {
		userid, err := strconv.ParseInt(*req.UserId, 10, 64)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErr, "user_id is not int")
			return
		}
		query = query.Where(device.UserID(userid))
	}
	if req.UpdateTime != nil {
		query = query.Where(device.UpdatedAt(*req.UpdateTime))
	}

	if req.State != nil {
		query = query.Where(device.StateEQ(device.State(*req.State)))
	}
	devices, err := query.All(c)
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, serializer.FrontendSerialize(devices))
	return
}

// NewDevice 新建一个新的待绑定的设备 GET，被客户端调用
func NewDevice(c *gin.Context) {
	var newDevice *cep_ent.Device
	var err error
	if txErr := db2.WithClient(nil, func(client *cep_ent.Client) error {
		newDevice, err = client.Device.Create().Save(c)
		if err != nil {
			logrus.Errorf("new device error : %v", err)
			return err
		}
		return nil
	}); txErr != nil {
		response.RespErrorWithMsg(c, code.ServerErr, txErr.Error())
		return
	}
	response.RespSuccess(c, serializer.FrontendSerialize(newDevice))
}

func SearchDevices(c *gin.Context) {
	// todo: 用户鉴权放到中间件去
	// 如果有 JWT，获取 JWT 中 user_id，没有也没事
	var userID int64
	token := c.GetHeader("Authorization")
	if token != "" {
		parts := strings.SplitN(token, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			mc, err := jwt.ParseToken(parts[1])
			if err != nil {
				response.RespErrorWithMsg(c, code.UnLogin, "token 失效")
				return
			}
			userID, err = strconv.ParseInt(mc.Info, 10, 64)
			if err != nil {
				response.RespErrorWithMsg(c, code.UnLogin, "token 内容错误")
				return
			}
		} else {
			response.RespErrorWithMsg(c, code.UnLogin, "token 内容错误")
			return
		}
	}
	// todo:--------------- 以上是鉴权 -----------------

	var req types.SearchDevicesReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	if userID != 0 {
		// 如果有 JWT，替换请求过滤中的 user_id
		req.UserID = &userID
	}
	query := db.DB.Device.Query().Where(device.DeletedAt(utils.ZeroTime)).Order(cep_ent.Desc(device.FieldUpdatedAt))
	if req.SerialNumber != nil {
		query = query.Where(device.SerialNumber(*req.SerialNumber))
	}
	if req.UserID != nil {
		query = query.Where(device.UserID(*req.UserID))
	}
	if req.Linking != nil {
		query = query.Where(device.Linking(*req.Linking))
	}
	devices, err := query.WithUser(func(userQuery *cep_ent.UserQuery) {
		userQuery.Where(user.DeletedAt(utils.ZeroTime))
	}).All(c)
	if err != nil {
		logrus.Errorf("db query devices with user err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	for _, _device := range devices {
		if _device.UserID != 0 {
			_device.SumCep, err = logic.GetDeviceWithUserProfit(c, nil, _device.ID, _device.UserID)
			if err != nil {
				response.RespErrorWithMsg(c, code.ServerErr, err.Error())
				return
			}
		}
	}
	res := serializer.FrontendSerialize(devices)
	// 隐藏 user hmac key
	for _, item := range res.([]interface{}) {
		edges := item.(map[string]interface{})["edges"]
		userEdge, ok := edges.(map[string]interface{})["user"]
		if ok {
			userEdge.(map[string]interface{})["secret"] = ""
		}
	}

	response.RespSuccess(c, res)
}

type GetDeviceReq struct {
	ID       int64 `uri:"id"`
	WithUser bool  `form:"with_user"`
}

func GetDevice(c *gin.Context) {
	var req GetDeviceReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	query := db.DB.Device.Query().Where(device.ID(req.ID), device.DeletedAt(utils.ZeroTime))
	if req.WithUser {
		query = query.WithUser(func(userQuery *cep_ent.UserQuery) {
			userQuery.Where(user.DeletedAt(utils.ZeroTime))
		})
	}
	_device, err := query.First(c)
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	data := serializer.FrontendSerialize(_device)
	if _device.UserID != 0 {
		_device.SumCep, err = logic.GetDeviceWithUserProfit(c, nil, _device.ID, _device.UserID)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErr, err.Error())
			return
		}
		edges := data.(map[string]interface{})["edges"]
		tempUser, ok := edges.(map[string]interface{})["user"]
		if ok {
			tempUser.(map[string]interface{})["secret"] = _device.Edges.User.Secret
		}
	}
	response.RespSuccess(c, data)
}

func UnbindDevice(c *gin.Context) {
	userid, err := common.GetUserId(c)
	if err != nil {
		return
	}
	strID := c.Param("id")
	deviceID, err := strconv.ParseInt(strID, 10, 64)
	if err != nil {
		response.RespErrorWithMsg(c, code.InvalidParams, "路径参数错误")
		return
	}
	_device, err := db.DB.Device.UpdateOneID(deviceID).Where(device.UserID(userid)).SetDeletedAt(time.Now()).Save(c)
	if err != nil {
		logrus.Errorf("update device %d deleted err: %v", deviceID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, serializer.FrontendSerialize(_device))
}

func UpdateDeviceBindingStatus(c *gin.Context) {
	var req types.UpdateDeviceBindingStatusReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	_device, err := handlers.UpdateDeviceBindingStatus(c, &handlers.UpdateDeviceBindingStatusOptions{
		Tx:            nil,
		ID:            req.ID,
		BindingStatus: req.BindingStatus,
	})
	if err != nil {
		errCode, ok := err.(code.MyCode)
		if ok {
			response.RespError(c, errCode)
		} else {
			response.RespErrorWithMsg(c, code.ServerErr, err.Error())
		}
		return
	}
	response.RespSuccess(c, serializer.FrontendSerialize(_device))
}

type GetPriceReq struct {
	Category    string `uri:"category"`
	Type        string `uri:"type"`
	Gpu         string `uri:"gpu"`
	BillingType string `form:"billing_type" json:"billing_type"`
}

func GetPrice(c *gin.Context) {
	var req GetPriceReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	query := db.DB.Price.Query().WithGpu().Where(
		price.MissionCategoryEQ(enums.MissionCategory(req.Category)),
		price.MissionTypeEQ(enums.MissionType(req.Type)),
		price.GpuVersionEQ(enums.GpuVersion(req.Gpu)),
	)
	if req.BillingType != "" {
		query = query.Where(price.MissionBillingTypeEQ(enums.MissionBillingType(req.BillingType)))
	}

	_price, err := query.First(c)
	if cep_ent.IsNotFound(err) {
		response.RespError(c, code.NotFound)
		return
	} else if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, serializer.FrontendSerialize(_price))
}

type GetGpuReq struct {
	Category string `uri:"category"`
	Type     string `uri:"types"`
}

type GetGpuResp struct {
	Gpus []interface{} `json:"gpus"`
}

func GetGpuWithCategoryAndType(c *gin.Context) {
	var req GetGpuReq
	err := c.ShouldBindUri(&req)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	all, err := db.DB.Price.Query().Where(price.MissionCategoryEQ(enums.MissionCategory(req.Category)), price.MissionTypeEQ(enums.MissionType(req.Type))).All(c)
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	gpuSet := hashset.New()
	for _, _price := range all {
		gpuSet.Add(_price.GpuVersion)
	}
	resp := GetGpuResp{
		Gpus: gpuSet.Values(),
	}
	response.RespSuccess(c, serializer.FrontendSerialize(resp))
}

type GetTypeReq struct {
	Category string `uri:"category"`
}

type GetTypeResp struct {
	Types []interface{} `json:"types"`
}

func GetTypeWithCategory(c *gin.Context) {
	var req GetTypeReq
	err := c.ShouldBindUri(&req)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	all, err := db.DB.Price.Query().Where(price.MissionCategoryEQ(enums.MissionCategory(req.Category))).All(c)
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	missionKindSet := hashset.New()
	for _, missionKind := range all {
		missionKindSet.Add(string(missionKind.MissionType))
	}
	resp := GetTypeResp{
		Types: missionKindSet.Values(),
	}
	response.RespSuccess(c, serializer.FrontendSerialize(resp))
}

type GetCategoryResp struct {
	Category []interface{} `json:"category"`
}

// GetCategory 获取到大类信息
func GetCategory(c *gin.Context) {
	all, err := db.DB.Price.Query().All(c)
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	category := hashset.New()
	for _, kind := range all {
		category.Add(kind.MissionCategory)
	}
	resp := GetCategoryResp{
		Category: category.Values(),
	}
	response.RespSuccess(c, serializer.FrontendSerialize(resp))
}

type GetGpuInfoReq struct {
	GpuName string `form:"gpu_name"`
}

func GetGpuInfo(c *gin.Context) {
	var req GetGpuInfoReq
	err := c.ShouldBind(&req)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	fmt.Println(req.GpuName)
	query := db.DB.Gpu.Query().Where(gpu.DeletedAt(utils.ZeroTime))
	if req.GpuName != "" {
		query = query.Where(gpu.VersionEQ(enums.GpuVersion(req.GpuName)))
	}
	gpus, err := query.All(c)
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, serializer.FrontendSerialize(gpus))
}

// CreateDeviceGpuMission 创建设备 GPU 信息
func CreateDeviceGpuMission(c *gin.Context) {
	var req types.CreateDeviceGpuMissionReq
	if err := c.ShouldBindUri(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	// 根据设备 ID 查询设备
	_device, err := db.DB.Device.Query().Where(device.ID(req.DeviceID), device.DeletedAt(utils.ZeroTime)).WithDeviceGpuMissions().First(c)
	if cep_ent.IsNotFound(err) {
		response.RespError(c, code.NotFound)
		return
	} else if err != nil {
		logrus.Errorf("query device %d err: %v", req.DeviceID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	logrus.Infof("device: %+v", _device)

	// 设置 req 的 map 信息
	reqSlotMap := make(map[int8]*types.GpuInfos)
	// 检查显卡设备是否存在，如果不存在则直接返回失败（显卡设备为后台常量数据）
	gpusName := hashset.New()
	reqGpusName := hashset.New()
	for _, _gpu := range req.GPUs {
		reqGpusName.Add(_gpu.Name)
		if len(_gpu.MissionID) == 0 {
			_gpu.GpuStatus = enums.GpuStatusFree
		} else {
			_gpu.GpuStatus = enums.GpuStatusBusy
		}
		reqSlotMap[_gpu.DeviceSlot] = &_gpu
	}
	gpus, err := db.DB.Gpu.Query().All(c)
	for _, _gpu := range gpus {
		gpusName.Add(string(_gpu.Version))
	}
	differenceGpu := reqGpusName.Difference(gpusName).Values()
	if len(differenceGpu) != 0 {
		response.RespErrorWithMsg(c, code.MyCode(40020), "存在不符合规范的 gpu")
		return
	}

	// 将数据库表更新到与传入状态相同
	if err = db2.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		deviceSlot := hashset.New()
		reqDeviceSlot := hashset.New()

		for _, _deviceGpu := range _device.Edges.DeviceGpuMissions {
			deviceSlot.Add(_deviceGpu.DeviceSlot)
		}
		for _, _reqDeviceGpu := range req.GPUs {
			reqDeviceSlot.Add(_reqDeviceGpu.DeviceSlot)
		}

		// 删除
		toDeleteDeviceSlot := deviceSlot.Difference(reqDeviceSlot)
		for _, toDeleteSlot := range toDeleteDeviceSlot.Values() {
			err = tx.DeviceGpuMission.Update().
				Where(devicegpumission.DeviceID(req.DeviceID),
					devicegpumission.DeviceSlot(toDeleteSlot.(int8)),
					devicegpumission.DeletedAtNEQ(utils.ZeroTime)).
				SetGpuStatus(enums.GpuStatusOffline).
				SetGpuID(
					tx.Gpu.Query().Where(gpu.VersionEQ(enums.GpuVersionUnknown)).FirstX(c).ID,
				).
				SetAbleMissionKind([]string{}).
				SetMissionID([]int64{}).
				Exec(c)
			if err != nil {
				return err
			}
		}

		// 新增
		toCreateDeviceSlot := reqDeviceSlot.Difference(deviceSlot)
		for _, toCreateSlot := range toCreateDeviceSlot.Values() {

			err = tx.DeviceGpuMission.Create().
				SetDeviceID(req.DeviceID).
				SetGpuID(
					tx.Gpu.Query().Where(gpu.VersionEQ(enums.GpuVersion(reqSlotMap[toCreateSlot.(int8)].Name))).FirstX(c).ID,
				).
				SetDeviceSlot(reqSlotMap[toCreateSlot.(int8)].DeviceSlot).
				SetMissionID(reqSlotMap[toCreateSlot.(int8)].MissionID).
				SetGpuStatus(reqSlotMap[toCreateSlot.(int8)].GpuStatus).
				SetAbleMissionKind(reqSlotMap[toCreateSlot.(int8)].MissionKind).
				SetMaxOnlineMission(reqSlotMap[toCreateSlot.(int8)].MaxOnlineMission).
				Exec(c)
			if err != nil {
				return err
			}
		}

		// 更新
		// fixme: 代码逻辑有待优化
		toUpdateDeviceSlot := reqDeviceSlot.Intersection(deviceSlot)
		for _, toUpdateSlot := range toUpdateDeviceSlot.Values() {

			i, err := tx.DeviceGpuMission.Update().
				Where(devicegpumission.DeviceID(req.DeviceID),
					devicegpumission.DeviceSlot(toUpdateSlot.(int8)),
					devicegpumission.DeletedAtEQ(utils.ZeroTime)).
				SetGpuStatus(reqSlotMap[toUpdateSlot.(int8)].GpuStatus).
				SetGpuID(
					tx.Gpu.Query().Where(gpu.VersionEQ(enums.GpuVersion(reqSlotMap[toUpdateSlot.(int8)].Name))).FirstX(c).ID,
				).
				SetUpdatedAt(time.Now()).
				SetAbleMissionKind(reqSlotMap[toUpdateSlot.(int8)].MissionKind).
				SetMissionID(reqSlotMap[toUpdateSlot.(int8)].MissionID).
				Save(c)
			logrus.Info(i)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}
}

type DeviceGpuMission struct {
	DeviceID       int64
	GpuName        string
	GpuMissionType string
}
