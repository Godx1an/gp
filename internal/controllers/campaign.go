package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/common"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/rechargecampaignrule"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"vpay/internal/db"
)

// ExistCampaign 活动是否存在
func ExistCampaign(c *gin.Context) {
	// fixme: 活动是否存在的判断依据应该是活动信息，当前设计不完善，根据营销账户余额判断
	// 查询营销账户余额

	marketingUser, err := db.DB.User.Query().WithCostAccount().Where(user.Phone("18888888888")).First(c)
	if cep_ent.IsNotFound(err) {
		response.RespErrorWithMsg(c, code.NotFound, "营销账户不存在")
		return
	} else if err != nil {
		logrus.Errorf("query marketing user err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	if marketingUser.Edges.CostAccount != nil {
		// 营销账户余额大于 0 则表示活动存在
		response.RespSuccess(c, marketingUser.Edges.CostAccount.PureCep > 0)
		return
	}

	response.RespSuccess(c, false)
}

// ListRechargeRule 充值规则列表
func ListRechargeRule(c *gin.Context) {
	rechargeCampaignRules, err := db.DB.RechargeCampaignRule.Query().Where(rechargecampaignrule.DeletedAtEQ(common.ZeroTime)).All(c)
	if err != nil {
		logrus.Errorf("query recharge campaign rule err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}
	response.RespSuccess(c, rechargeCampaignRules)
}
