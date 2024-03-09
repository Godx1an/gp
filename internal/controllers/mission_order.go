package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/enumcondition"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/enummissionstatus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/mission"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/missionorder"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/response"
	"strconv"
	"time"
	"vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/handlers"
	"vpay/internal/middleware"
	"vpay/internal/myredis"
	"vpay/internal/types"
	"vpay/utils"
	"vpay/utils/db_utils"
)

func SearchMissionOrdersCep(c *gin.Context) {
	_, err := getUserId(c)
	if err != nil {
		return
	}
	var req types.SearchMissionOrdersCepReq
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	missionConsumeOrdersCep, err := handlers.SearchMissionOrdersCep(c, &handlers.SearchMissionOrdersCepOptions{
		Tx:         nil,
		IDs:        req.IDs,
		MissionIDs: req.MissionIDs,
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
	response.RespSuccess(c, serializer.FrontendSerialize(missionConsumeOrdersCep))
}

func SearchMissionOrdersSub(c *gin.Context) {
	var req types.SearchMissionOrdersSubReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	missionConsumeOrderSubInfos, err := handlers.SearchMissionsSubInfo(c, &handlers.SearchMissionOrdersSubInfoOptions{
		Tx:         nil,
		MissionIDs: req.MissionIDs,
	})
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	response.RespSuccess(c, serializer.FrontendSerialize(missionConsumeOrderSubInfos))
}

func SearchMissionOrders(c *gin.Context) {
	// 用户凭证拿到 user_id
	userID, err := getUserId(c)
	if err != nil {
		return
	}
	var req types.SearchMissionOrdersReq
	if err := c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	var nillableBatchNumber *string
	if req.BatchNumber != "" {
		nillableBatchNumber = &req.BatchNumber
	}
	// 检查前端传入的 int64 不是 0
	var missionIDs []int64
	for _, item := range req.MissionIDs {
		if item != 0 {
			missionIDs = append(missionIDs, item)
		}
	}
	var missionOrderStatuses []enums.MissionOrderStatus
	for _, item := range req.MissionOrderStatus {
		if item != "" {
			missionOrderStatuses = append(missionOrderStatuses, item)
		}
	}
	var missionTypes []enums.MissionType
	for _, item := range req.Types {
		if item != "" {
			missionTypes = append(missionTypes, item)
		}
	}
	// 处理前端专用的参数转化规则
	var enumConditions []*cep_ent.EnumCondition
	if req.FrontType != "" {
		// 去数据库中获取对应的规则
		enumConditions, err = db.DB.EnumCondition.Query().Where(enumcondition.FrontType(req.FrontType)).All(c)
		if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, err.Error())
			return
		}
	}
	var enumMissionStatuses []*cep_ent.EnumMissionStatus
	// 多一层逻辑，处理 closing 的 mission
	var nillableMissionIsClosing *bool
	for _, frontStatus := range req.FrontStatuses {
		True := true
		False := false
		if frontStatus != "" {
			switch frontStatus {
			case "waiting":
				nillableMissionIsClosing = &False
			case "doing":
				nillableMissionIsClosing = &False
			case "succeed":
				nillableMissionIsClosing = &True
			}
			tempEnumMissionStatuses, err := db.DB.EnumMissionStatus.Query().Where(enummissionstatus.FrontStatus(frontStatus)).All(c)
			if err != nil {
				response.RespErrorWithMsg(c, code.ServerErrDB, err.Error())
				return
			}
			enumMissionStatuses = append(enumMissionStatuses, tempEnumMissionStatuses...)
		}
	}
	missionOrders, totalCount, err := handlers.SearchMissionOrders(c, &handlers.SearchMissionOrdersOptions{
		Tx: nil,
		PaginateReq: common_types.PaginateReq{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
		BatchNumber:          nillableBatchNumber,
		MissionIDs:           missionIDs,
		UserID:               &userID,
		MissionTypes:         missionTypes,
		MissionOrderStatuses: missionOrderStatuses,
		EnumConditions:       enumConditions,
		EnumMissionStatuses:  enumMissionStatuses,
		StartedAtSort:        req.StartedAtSort,
		FinishedAtSort:       req.FinishedAtSort,
		CepSort:              req.CepSort,
		WithMission:          req.WithMission,
		StartedAtGte:         req.StartedAtGte,
		FinishedAtLte:        req.FinishedAtLte,
		IsTime:               req.IsTime,
		MissionIsClosing:     nillableMissionIsClosing,
		HaveSub:              req.HaveSub,
		MissionBillingType:   req.MissionBillingType,
	})
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, err.Error())
		return
	}
	dataList := serializer.FrontendSerialize(missionOrders)
	enumConditions, err = db.DB.EnumCondition.Query().All(c)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErrDB, err.Error())
		return
	}
	for i, data := range dataList.([]interface{}) {
		tempType := data.(map[string]interface{})["mission_type"].(string)
		tempCallWay := data.(map[string]interface{})["call_way"].(string)
		foundFrontType := false
		if _, ok := data.(map[string]interface{})["edges"].(map[string]interface{})["mission"]; ok && data.(map[string]interface{})["edges"].(map[string]interface{})["mission"].(map[string]interface{})["call_back_url"] == "" {
			data.(map[string]interface{})["edges"].(map[string]interface{})["mission"].(map[string]interface{})["result_url"] = missionOrders[i].Edges.Mission.ResultUrls
			data.(map[string]interface{})["edges"].(map[string]interface{})["mission"].(map[string]interface{})["body"] = "deprecated"
		} else if ok {
			data.(map[string]interface{})["edges"].(map[string]interface{})["mission"].(map[string]interface{})["result_url"] = []string{}
			data.(map[string]interface{})["edges"].(map[string]interface{})["mission"].(map[string]interface{})["body"] = "deprecated"
		}

		first, err := db.DB.EnumMissionStatus.Query().Where(enummissionstatus.MissionTypeEQ(string(missionOrders[i].MissionType)), enummissionstatus.MissionStatusEQ(string(missionOrders[i].Status))).First(c)
		if cep_ent.IsNotFound(err) {
			data.(map[string]interface{})["front_status"] = enums.MissionOrderStatusUnknown
		} else if err != nil {
			response.RespErrorWithMsg(c, code.ServerErrDB, err.Error())
			return
		} else {
			data.(map[string]interface{})["front_status"] = first.FrontStatus
		}

		for _, enumCondition := range enumConditions {
			if enumCondition.MissionType == tempType && enumCondition.MissionCallWay == tempCallWay {
				data.(map[string]interface{})["front_type"] = enumCondition.FrontType
				foundFrontType = true
				break
			}
		}
		if !foundFrontType {
			data.(map[string]interface{})["front_type"] = "API"
		}
	}
	response.RespSuccessPagination(c, req.PageIndex, req.PageSize, int64(totalCount), dataList)
}

type SearchMissionBatchesReq struct {
	common_types.PaginateReq
	Type *enums.MissionType `form:"type"`
}

func SearchMissionBatches(c *gin.Context) {
	// 用户凭证拿到 user_id
	userID, err := getUserId(c)
	if err != nil {
		return
	}
	var req SearchMissionBatchesReq
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	// 直接搜索用户创建的批次条，按创建时间逆序排序
	missionBatches, totalCount, err := handlers.SearchMissionBatches(c, &handlers.SearchMissionBatchesOptions{
		Tx:     nil,
		UserID: &userID,
		PaginateReq: common_types.PaginateReq{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
		Type: req.Type,
	})
	if err != nil {
		errCode, ok := err.(code.MyCode)
		if ok {
			response.RespError(c, errCode)
			return
		} else {
			response.RespErrorWithMsg(c, code.ServerErr, err.Error())
			return
		}
	}
	response.RespSuccessPagination(c, req.PageIndex, req.PageSize, int64(totalCount), serializer.FrontendSerialize(missionBatches))
	return
}

func FetchMissionOrderStatus(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	defer conn.Close()

	// 计时器，没输入就关闭
	var timer *time.Timer
	timer = time.NewTimer(time.Minute * 10)
	closeChannel := make(chan struct{})
	go func(_timer *time.Timer) {
		select {
		case <-_timer.C:
			respData, _ := json.Marshal(map[string]interface{}{
				"msg":  "超时无输入，已关闭链接",
				"type": "close",
			})
			if err = conn.WriteMessage(websocket.TextMessage, respData); err != nil {
				fmt.Printf("%v", err)
			}
			closeChannel <- struct{}{}
			return
		}
	}(timer)

	go func() {
		// 第一步先读前端发来的 Authorization
		_, p, err := conn.ReadMessage()
		if err != nil {
			closeChannel <- struct{}{}
			return
		}
		var req struct {
			Authorization string `json:"authorization"`
		}
		if err = json.Unmarshal(p, &req); err != nil {
			closeChannel <- struct{}{}
			return
		}
		userID, err, _ := middleware.TokenToUserID(req.Authorization)
		if err != nil {
			closeChannel <- struct{}{}
			return
		}
		// 前端有请求就 reset timer
		timer.Reset(time.Minute * 10)

		// 开协程接收前端的后续请求保持链接
		go func() {
			for {
				_, p, err = conn.ReadMessage()
				if err != nil {
					closeChannel <- struct{}{}
					return
				}
				if err = json.Unmarshal(p, &req); err != nil {
					closeChannel <- struct{}{}
					return
				}
				// 并且可以更新 userID
				userID, err, _ = middleware.TokenToUserID(req.Authorization)
				if err != nil {
					closeChannel <- struct{}{}
					return
				}
				// 前端有请求就 reset timer
				timer.Reset(time.Minute * 10)
			}
		}()

		// 开始监听是否有任务被强制关闭
		for {
			msg, err := myredis.UserMissionSub.ReceiveMessage(c)
			if err != nil {
				closeChannel <- struct{}{}
				return
			}
			var publishCloseUserMissionData types.PublishCloseUserMissionData
			if err = json.Unmarshal([]byte(msg.Payload), &publishCloseUserMissionData); err != nil {
				fmt.Printf("reading from redis: %v", err)
				closeChannel <- struct{}{}
				return
			}

			// 当前用户出现余额预警
			if publishCloseUserMissionData.UserID == userID && publishCloseUserMissionData.MissionID != 0 {
				resData, _ := json.Marshal(map[string]interface{}{
					"type":       "force_close",
					"msg":        fmt.Sprintf("任务 %d 已被强制关闭", publishCloseUserMissionData.MissionID),
					"mission_id": fmt.Sprintf("%d", publishCloseUserMissionData.MissionID),
				})
				if err = conn.WriteMessage(websocket.TextMessage, resData); err != nil {
					response.RespErrorWithMsg(c, code.ServerErr, err.Error())
					closeChannel <- struct{}{}
					return
				}
			}
		}
	}()

	select {
	case <-closeChannel:
		return
	}
}

func AllMissionsDoing(c *gin.Context) {
	orders, err := db.DB.MissionOrder.Query().
		Where(missionorder.StatusEQ(enums.MissionOrderStatusSupplying),
			missionorder.HasMissionWith(mission.DeletedAt(utils.ZeroTime))).
		WithMission(func(missionQuery *cep_ent.MissionQuery) {
			missionQuery.Where(mission.DeletedAt(utils.ZeroTime))
		}).
		All(c)
	if err != nil {
		logrus.Errorf("get all mission consume order doing err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, serializer.FrontendSerialize(orders))
}

type UpdateMissionOrdersReq struct {
	MissionIDs []string             `json:"mission_ids"`
	CallWay    enums.MissionCallWay `json:"call_way"`
}

// UpdateMissionOrders 目前需要被前端修改的订单属性就是调用方式
func UpdateMissionOrders(c *gin.Context) {
	_, err := getUserId(c)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	var req UpdateMissionOrdersReq
	if err = c.ShouldBindJSON(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}

	// 处理参数
	var realMissionIDs []int64
	for _, item := range req.MissionIDs {
		if item != "" {
			tempID, err := strconv.ParseInt(item, 10, 64)
			if err != nil {
				response.RespErrorInvalidParams(c, err)
			}
			realMissionIDs = append(realMissionIDs, tempID)
		}
	}

	// 执行逻辑，简单修改 call_way
	if txErr := db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		update := tx.MissionOrder.Update().Where(missionorder.DeletedAt(utils.ZeroTime), missionorder.MissionIDIn(realMissionIDs...))
		if req.CallWay.Ptr() != nil {
			update.SetCallWay(req.CallWay)
		}
		if err = update.Exec(c); err != nil {
			return err
		}
		return nil
	}); txErr != nil {
		response.DynamicRespErr(c, err)
		return
	}
	response.RespSuccess(c, nil)
}

// AddMissionOrder 新增任务订单（续费任务）
func AddMissionOrder(c *gin.Context) {
	var req types.AddMissionOrderReq
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

	missionID, err := strconv.ParseInt(req.MissionID, 10, 64)
	if err != nil {
		logrus.Errorf("parse int failed, err: %v", err)
		response.RespErrorInvalidParams(c, errors.New("mission_id 格式错误"))
		return
	}

	missionOrder, err := handlers.AddMissionOrder(c, handlers.AddMissionOrderOptions{
		UserID:             userId,
		MissionID:          missionID,
		BuyDuration:        req.BuyDuration,
		MissionBillingType: req.MissionBillingType,
	})
	if err != nil {
		DynamicResponseErr(c, err)
		return
	}

	response.RespSuccess(c, missionOrder)
}
