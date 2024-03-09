package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/device"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/frpcinfo"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/frpsinfo"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"vpay/internal/db"
	"vpay/internal/types"
	"vpay/utils"
	"vpay/utils/db_utils"
)

// FrpClientGetPorts 根据 frpc 端口获取 frps 开放端口
func FrpClientGetPorts(c *gin.Context) {
	var req types.FrpClientGetPortsReq
	if err := c.ShouldBind(&req); err != nil {
		logrus.Errorf("bind request failed, err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	// 设备是否存在
	exist, err := db.DB.Device.Query().Where(device.IDEQ(req.DeviceID)).Exist(c)
	if err != nil {
		logrus.Errorf("db exist device id(%d) failed, err: %v", req.DeviceID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	if !exist {
		logrus.Warningf("device(%d) not exist", req.DeviceID)
		response.RespErrorWithMsg(c, code.NotFound, "device not exist")
		return
	}

	// 查询绑定了该 tag 的数据
	tagFrpcInfos, err := db.DB.FrpcInfo.Query().Where(frpcinfo.TagEQ(req.FrpcTag)).All(c)
	if err != nil {
		logrus.Errorf("db query frpc info failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	// 判断该 tag 是否已经绑定了其他设备
	for _, tagFrpcInfo := range tagFrpcInfos {
		if tagFrpcInfo.DeviceID != req.DeviceID {
			logrus.Warningf("frpc tag(%s) already bind to device(%d)", req.FrpcTag, tagFrpcInfo.DeviceID)
			response.RespErrorWithMsg(c, code.InvalidParams, "frpc tag already bind to other device")
			return
		}
	}

	// 随机选择 frps V1 or V2（为了分散带宽压力），单数选 V1，双数选 V2
	//var frpsType = types.TypeFrpsV1
	//if rand.Intn(10)%2 == 0 {
	//	// 双数
	//	frpsType = types.TypeFrpsV2
	//}
	// fixme： 0226 先全部用新服务
	frpsType := types.TypeFrpsV3

	// 查询该设备该本地端口已经分配了远程端口的数据 -- 20231225 使用 v2 frps
	commonExistedFrpcInfos, err := db.DB.FrpcInfo.Query().WithFrpsInfo().Where(frpcinfo.HasFrpsInfoWith(frpsinfo.TypeEQ(frpsType)), frpcinfo.DeviceIDEQ(req.DeviceID), frpcinfo.TagEQ(req.FrpcTag), frpcinfo.LocalPortIn(req.FrpcLocalPorts...)).All(c)
	if err != nil {
		logrus.Errorf("db query common frpc info failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	// 已经存在的本地端口不需要重新分配
	needCommonRemotePorts := make([]int, len(req.FrpcLocalPorts))
	copy(needCommonRemotePorts, req.FrpcLocalPorts)
	for _, existedFrpcInfo := range commonExistedFrpcInfos {
		if utils.IsIntInSlice(existedFrpcInfo.LocalPort, needCommonRemotePorts) {
			needCommonRemotePorts = utils.RemoveValueInIntSlice(needCommonRemotePorts, existedFrpcInfo.LocalPort)
		}
	}
	logrus.Infof("need common remote ports: %+v", needCommonRemotePorts)
	// 需要的端口数量（公有）
	needCommonPorts := len(needCommonRemotePorts)

	commonFrpcInfos := make([]*cep_ent.FrpcInfo, 0)
	if needCommonPorts > 0 {
		// 查询未使用的端口（公有） -- 20231225 使用 v2 frps
		commonFrpcInfos, err = db.DB.FrpcInfo.Query().WithFrpsInfo().Where(frpcinfo.HasFrpsInfoWith(frpsinfo.TypeEQ(frpsType)), frpcinfo.IsUsingEQ(false)).Limit(needCommonPorts).All(c)
		if err != nil {
			logrus.Errorf("db query frpc info failed, err: %v", err)
			response.RespError(c, code.ServerErrDB)
			return
		}
		// 判断是否有足够的端口（公有）
		if len(commonFrpcInfos) < needCommonPorts {
			response.RespErrorWithMsg(c, code.NotEnough, "not enough public remote ports")
			return
		}
	}

	commonRespFrpcInfo := make([]*cep_ent.FrpcInfo, 0)
	if err = db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		// 更新 public frpc 端口信息
		for idx, frpcInfo := range commonFrpcInfos {
			err = tx.FrpcInfo.UpdateOneID(frpcInfo.ID).
				SetTag(req.FrpcTag).
				SetLocalPort(needCommonRemotePorts[idx]).
				SetIsUsing(true).
				SetDeviceID(req.DeviceID).
				Exec(c)
			if err != nil {
				return errors.Errorf("db update frpc info failed, err: %v", err)
			}
			newFrpcInfo, err := tx.FrpcInfo.Query().WithFrpsInfo().Where(frpcinfo.ID(frpcInfo.ID)).First(c)
			if err != nil {
				return errors.Errorf("db get frpc info failed, err: %v", err)
			}
			commonRespFrpcInfo = append(commonRespFrpcInfo, newFrpcInfo)
		}
		return nil
	}); err != nil {
		logrus.Errorf("db with tx failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	// 将已经存在的 common frpc 端口信息也返回
	commonRespFrpcInfo = append(commonRespFrpcInfo, commonExistedFrpcInfos...)

	var resp types.FrpClientGetPortsResp
	resp.CommonPorts = commonRespFrpcInfo

	response.RespSuccess(c, resp)
}

// FrpClientRecyclePorts 回收 frpc 端口
func FrpClientRecyclePorts(c *gin.Context) {
	var req types.FrpClientRecyclePortsReq
	if err := c.ShouldBind(&req); err != nil {
		logrus.Errorf("bind request failed, err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	if err := db.DB.FrpcInfo.Update().
		Where(
			frpcinfo.DeviceIDEQ(req.DeviceID),
			frpcinfo.LocalPortIn(req.FrpcLocalPorts...),
		).
		SetTag("").
		SetLocalPort(0).
		SetIsUsing(false).
		SetDeviceID(0).Exec(c); err != nil {
		logrus.Errorf("db update frpc ports failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, nil)
}

// FrpClientGetPrivatePort 获取 frps 开放私有端口
func FrpClientGetPrivatePort(c *gin.Context) {
	var req types.FrpClientGetPrivatePortReq
	if err := c.ShouldBind(&req); err != nil {
		logrus.Errorf("bind request failed, err: %v", err)
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("req: %+v", req)

	// 设备是否存在
	exist, err := db.DB.Device.Query().Where(device.IDEQ(req.DeviceID)).Exist(c)
	if err != nil {
		logrus.Errorf("db exist device id(%d) failed, err: %v", req.DeviceID, err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	if !exist {
		logrus.Warningf("device(%d) not exist", req.DeviceID)
		response.RespErrorWithMsg(c, code.NotFound, "device not exist")
		return
	}

	// 查询绑定了该 tag 的数据
	tagFrpcInfos, err := db.DB.FrpcInfo.Query().Where(frpcinfo.TagEQ(req.FrpcTag)).All(c)
	if err != nil {
		logrus.Errorf("db query frpc info failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	// 判断该 tag 是否已经绑定了其他设备
	for _, tagFrpcInfo := range tagFrpcInfos {
		if tagFrpcInfo.DeviceID != req.DeviceID {
			logrus.Warningf("frpc tag(%s) already bind to device(%d)", req.FrpcTag, tagFrpcInfo.DeviceID)
			response.RespErrorWithMsg(c, code.InvalidParams, "frpc tag already bind to other device")
			return
		}
	}

	// 配备私有端口（私有端口默认只需要分配 22 端口）
	// 查询该设备该本地端口已经分配了远程端口的数据（私有）
	privateFrpcInfo, err := db.DB.FrpcInfo.Query().Where(frpcinfo.HasFrpsInfoWith(frpsinfo.TypeEQ("private")), frpcinfo.DeviceIDEQ(req.DeviceID), frpcinfo.LocalPortEQ(22)).First(c)
	if cep_ent.IsNotFound(err) {
		// 不存在，需要分配
		// 查询未使用的端口（私有）
		privateFrpcInfo, err = db.DB.FrpcInfo.Query().Where(frpcinfo.HasFrpsInfoWith(frpsinfo.TypeEQ("private")), frpcinfo.IsUsingEQ(false)).First(c)
		if cep_ent.IsNotFound(err) {
			logrus.Errorf("db query private frpc info failed, err: %v", err)
			response.RespErrorWithMsg(c, code.NotEnough, "not enough private remote ports")
			return
		} else if err != nil {
			logrus.Errorf("db query private frpc info failed, err: %v", err)
			response.RespError(c, code.ServerErrDB)
			return
		}
	} else if err != nil {
		logrus.Errorf("db query private frpc info failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	logrus.Infof("private frpc info: %+v", privateFrpcInfo)

	// 更新 private frpc 端口信息
	if err = db.DB.FrpcInfo.UpdateOneID(privateFrpcInfo.ID).SetTag(req.FrpcTag).SetIsUsing(true).SetDeviceID(req.DeviceID).Exec(c); err != nil {
		logrus.Errorf("db update private frpc info failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, privateFrpcInfo)
}
