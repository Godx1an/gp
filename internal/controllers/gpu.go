package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/common"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/devicegpumission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/gpu"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"slices"
	"vpay/internal/db"
	"vpay/internal/types"
)

// GpuFreeCountByVersion 获取某一版本 gpu 的空闲数量
func GpuFreeCountByVersion(c *gin.Context) {
	gpuVersion := c.Param("gpu_version")

	logrus.Debugf("req: %+v", gpuVersion)

	var dbGpuVersion enums.GpuVersion
	if !slices.Contains(dbGpuVersion.Values(), gpuVersion) {
		response.RespErrorWithMsg(c, code.InvalidParams, "请输入正确的 gpu 版本名")
		return
	}

	// 根据 gpu 名查询当前空闲的 gpu 数量
	gpuFreeCount, err := db.DB.DeviceGpuMission.Query().Where(
		devicegpumission.DeletedAt(common.ZeroTime),
		devicegpumission.GpuStatusEQ(enums.GpuStatusFree),
		devicegpumission.HasGpuWith(gpu.VersionEQ(enums.GpuVersion(gpuVersion))),
	).Count(c)
	if err != nil {
		response.RespError(c, code.ServerErrDB)
		return
	}

	var respData types.GpuFreeCountByVersionResp
	respData.GpuVersion = gpuVersion
	respData.Count = gpuFreeCount

	response.RespSuccess(c, respData)
}
