package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/gin-gonic/gin"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkbitable "github.com/larksuite/oapi-sdk-go/v3/service/bitable/v1"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/common"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/device"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/devicegpumission"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"io/ioutil"
	"net/http"
	"vpay/internal/db"
	"vpay/internal/types"
)

type PostData struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type LarkTokenResp struct {
	Code   int64  `json:"code"`
	Expire int64  `json:"expire"`
	Msg    string `json:"msg"`
	Token  string `json:"tenant_access_token"`
}

type LarkUserTokenResp struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	Scope            string `json:"scope"`
}

const (
	appToken        = `RvFabsjQWaXNXNsETHjck1v5n8d`
	tableID         = `tblL2UhcgVN7H5gg`
	userAccessToken = "u-dDc_OjnFt4HF1S7iHjftbP1gifPNhkpxiwG0l1y20JDM"
)

func LarkDeviceMissionList(ctx *gin.Context) {
	// 获取 tenant_access_token
	url := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal/"
	var postData PostData
	postData.AppID = "cli_a5fc7e246f3b1013"
	postData.AppSecret = "9vUJz0yTqECtLA5vdQ1P6eKtYirYpuX7"
	contentType := "application/json; charset=utf-8"

	jsonStr, err := json.Marshal(postData)
	if err != nil {
		logrus.Errorf("failed to get marshal, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		logrus.Errorf("failed to new request, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}
	req.Header.Set("Content-Type", contentType)
	head := "Bearer "
	req.Header.Set("Authorization", fmt.Sprintf("%s%s", head, appToken))
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorf("failed to send request, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}
	body, _ := ioutil.ReadAll(resp.Body)
	var larkTokenResp LarkTokenResp
	err = json.Unmarshal(body, &larkTokenResp)
	if err != nil {
		logrus.Errorf("failed to unmarshal, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}

	tenantAccessToken := fmt.Sprintf("Bearer %v", larkTokenResp.Token)
	// userAccessToken := larkUserTokenResp.AccessToken
	// 创建 Client
	// 如需SDK自动管理租户Token的获取与刷新，可调用lark.WithEnableTokenCache(true)进行设置
	larkClient := lark.NewClient(postData.AppID, postData.AppSecret)

	listReq := larkbitable.NewListAppTableRecordReqBuilder().
		AppToken(appToken).
		TableId(tableID).
		Build()

	// 发起请求查询所有数据
	listResp, err := larkClient.
		Bitable.AppTableRecord.
		List(context.Background(), listReq, larkcore.WithTenantAccessToken(tenantAccessToken))
	// 处理错误
	if err != nil {
		logrus.Errorf("failed to query the list, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}
	// 服务端错误处理
	if !listResp.Success() {
		logrus.Errorf("failed to query the list, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}

	// 获取记录 ID
	var toDeleteIDs []string
	for _, item := range listResp.Data.Items {
		toDeleteIDs = append(toDeleteIDs, *item.RecordId)
	}

	if len(toDeleteIDs) != 0 {
		// 存在数据时才进行更新
		delReq := larkbitable.NewBatchDeleteAppTableRecordReqBuilder().
			AppToken(appToken).
			TableId(tableID).
			Body(larkbitable.NewBatchDeleteAppTableRecordReqBodyBuilder().
				Records(toDeleteIDs).
				Build()).
			Build()
		delResp, err := larkClient.
			Bitable.AppTableRecord.
			BatchDelete(context.Background(), delReq, larkcore.WithTenantAccessToken(tenantAccessToken))
		// 处理错误
		if err != nil {
			logrus.Errorf("failed to delete the list, err: %v", err)
			response.RespError(ctx, code.ServerErr)
			return
		}
		// 服务端错误处理
		if !delResp.Success() {
			logrus.Errorf("failed to delete the list, err: %v", err)
			response.RespError(ctx, code.ServerErr)
			return
		}
	}

	devices, err := db.DB.Device.Query().
		Where(device.DeletedAtEQ(common.ZeroTime)).
		WithDeviceGpuMissions(func(query *cep_ent.DeviceGpuMissionQuery) {
			query.Select(devicegpumission.FieldAbleMissionKind).
				Order(cep_ent.Asc(devicegpumission.FieldDeviceSlot))
		}).
		All(ctx)
	if err != nil {
		logrus.Errorf("failed to query the device, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	mapArray := make([]map[string]interface{}, 0, len(devices))
	for _, _device := range devices {
		tempMap := make(map[string]interface{})
		tempMap["设备名称"] = _device.Name
		serializeID := serializer.FrontendSerialize(_device.ID)
		tempMap["设备ID"] = serializeID
		tempMap["可接任务"] = ParsingTask(_device)
		mapArray = append(mapArray, tempMap)
	}

	// 创建请求
	records := make([]*larkbitable.AppTableRecord, 0, len(mapArray))
	for _, m := range mapArray {
		record := larkbitable.NewAppTableRecordBuilder().Fields(m).Build()
		records = append(records, record)
	}

	addReq := larkbitable.NewBatchCreateAppTableRecordReqBuilder().
		AppToken(appToken).
		TableId(tableID).
		Body(larkbitable.NewBatchCreateAppTableRecordReqBodyBuilder().
			Records(records).
			Build()).
		Build()

	// 发起请求
	addResp, err := larkClient.
		Bitable.AppTableRecord.
		BatchCreate(context.Background(), addReq, larkcore.WithTenantAccessToken(tenantAccessToken))

	// 处理错误
	if err != nil {
		logrus.Errorf("failed to add the list, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}

	// 服务端错误处理
	if !addResp.Success() {
		logrus.Errorf("failed to delete the list, err: %v", err)
		response.RespError(ctx, code.ServerErr)
		return
	}

	response.RespSuccess(ctx, code.Success)
}

// ParsingTask 解析可完成任务
func ParsingTask(device *cep_ent.Device) (res string) {
	appTypeMap := types.InitMissionType()
	appTypeSet := hashset.New()
	if len(device.Edges.DeviceGpuMissions) <= 0 || device.Edges.DeviceGpuMissions == nil {
		return res
	}
	for _, ableMissionKind := range device.Edges.DeviceGpuMissions[0].AbleMissionKind {
		appTypeSet.Add(appTypeMap[ableMissionKind])
	}
	for i, task := range appTypeSet.Values() {
		if i == 0 {
			res = fmt.Sprintf("%s%s", res, task)
		} else {
			res = fmt.Sprintf("%s%s%s", res, ", ", task)
		}
	}
	return res
}
