package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/common_types"
	"github.com/stark-sim/serializer/response"
	"strconv"
	"vpay/internal/common"
	"vpay/internal/handlers"
	"vpay/internal/types"
)

// GetOnesBills 获取个人的流水情况
func GetOnesBills(c *gin.Context) {
	userID, err := common.GetUserId(c)
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	var req types.GetBillReq
	err = c.ShouldBind(&req)
	if err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	var billID int64
	if len(req.BillID) != 0 {
		billID, err = strconv.ParseInt(req.BillID, 10, 64)
		if err != nil {
			response.RespError(c, code.ServerErr)
			return
		}
	} else {
		billID = 0
	}
	bills, totalCount, err := handlers.GetBillsByCondition(c, &handlers.GetBillsByConditionOptions{
		Tx:            nil,
		BillID:        billID,
		BillType:      req.BillType,
		BillWay:       req.BillWay,
		BillTimeBegin: req.BillTimeBegin,
		BillTimeEnd:   req.BillTimeEnd,
		BillSymbolID:  req.BillSymbolID,
		PageIndex:     req.PageIndex,
		PageSize:      req.PageSize,
		UserID:        userID,
		BillQueryType: req.BillQueryType,
	})
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}

	var billsResp []*BillsResp
	for i, _ := range bills {

		billResp := &BillsResp{}

		billResp.Bill = bills[i]
		if billResp.TargetUserID == userID {
			billResp.IsAdd = true
		}
		billsResp = append(billsResp, billResp)

	}

	// fixme: serializer.FrontendSerialize 方式规范返回 json 可以在后期通过 json target 来进行规范，后期可对 serializer.FrontendSerialize 删除
	billsData := serializer.FrontendSerialize(billsResp)
	// 将返回信息规范为分页方式
	resp := common_types.PaginateResp{
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
		Total:     totalCount,
		List:      billsData,
	}
	response.RespSuccess(c, resp)
}

type BillsResp struct {
	*cep_ent.Bill

	IsAdd bool `json:"is_add"`
}
