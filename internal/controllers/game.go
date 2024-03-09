package controllers

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/common"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/lotto"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/lottoprize"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/lottorecord"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/lottousercount"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/cephalon-ent/pkg/enums"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"slices"
	"strconv"
	"time"
	"vpay/internal/db"
	"vpay/internal/handlers"
	"vpay/internal/logic"
	"vpay/internal/myredis"
	"vpay/internal/types"
	"vpay/utils"
	"vpay/utils/db_utils"
)

// GetUserPrizeWheelRemainCount 用户剩余抽奖次数
func GetUserPrizeWheelRemainCount(c *gin.Context) {
	// 用户凭证拿到 user_id
	userID, err := getUserId(c)
	if err != nil {
		return
	}
	var req types.GetUserPrizeWheelRemainCountReq
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("user: %d, req: %+v", userID, req)

	lottoUserCount, err := db.DB.LottoUserCount.Query().Where(lottousercount.DeletedAt(utils.ZeroTime), lottousercount.LottoID(req.LottoId), lottousercount.UserID(userID)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespSuccess(c, 0)
			return
		}
		response.RespError(c, code.ServerErrDB)
		return
	}

	response.RespSuccess(c, lottoUserCount.RemainLottoCount)
}

// PrizeWheelPlay 抽奖
func PrizeWheelPlay(c *gin.Context) {
	// 用户凭证拿到 user_id
	userID, err := getUserId(c)
	if err != nil {
		return
	}

	now := time.Now()

	var req types.PrizeWheelPlayReq
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("user: %d, req: %+v", userID, req)

	// 查询活动信息
	_lotto, err := db.DB.Lotto.Query().WithLottoPrizes(func(lottoPrizeQuery *cep_ent.LottoPrizeQuery) {
		lottoPrizeQuery.Where(lottoprize.DeletedAt(utils.ZeroTime))
	}).Where(lotto.DeletedAt(utils.ZeroTime), lotto.ID(req.LottoId), lotto.StartedAtLTE(now), lotto.EndedAtGTE(now)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespErrorWithMsg(c, code.NotFound, "不在活动有效期内")
			return
		}
		logrus.Errorf("db query lotto failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	// 查询用户在这个活动的抽奖次数
	lottoUserCount, err := db.DB.LottoUserCount.Query().WithUser(func(userQuery *cep_ent.UserQuery) {
		userQuery.Where(user.DeletedAt(utils.ZeroTime))
	}).Where(lottousercount.DeletedAt(utils.ZeroTime), lottousercount.LottoID(req.LottoId), lottousercount.UserID(userID)).First(c)
	if err != nil {
		if cep_ent.IsNotFound(err) {
			response.RespErrorWithMsg(c, code.NotFound, "抽奖次数不足")
			return
		}
		logrus.Errorf("db query lotto user count failed, err: %v", err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	if lottoUserCount.RemainLottoCount <= 0 {
		response.RespErrorWithMsg(c, code.NotEnough, "抽奖次数不足")
		return
	}

	// 查询用户是否是不需要限制每天抽奖次数的用户
	notLimitUsers, err := myredis.Client.LRange(c, types.RedisKeyLottoGameNotLimitUser, 0, -1).Result()
	if err != nil {
		logrus.Errorf("redis lrange not limit user failed, err: %v", err)
		response.RespError(c, code.ServerErrCache)
		return
	}
	if !slices.Contains(notLimitUsers, strconv.FormatInt(userID, 10)) {
		// 不在白名单内，需要判断今天抽奖次数是否超上限
		// 查询该用户在该活动今天抽奖的次数
		now := time.Now()
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		todayGameCount, err := db.DB.LottoRecord.Query().Where(
			lottorecord.DeletedAt(utils.ZeroTime),
			lottorecord.LottoID(req.LottoId),
			lottorecord.UserID(userID),
			//lottorecord.CreatedAtGT(types.TodayStart),
			lottorecord.CreatedAtGT(todayStart),
		).Count(c)
		if err != nil {
			logrus.Errorf("db get user today lotto record count failed, err: %v", err)
			response.RespError(c, code.ServerErrDB)
			return
		}

		if todayGameCount > types.LottoGamePrizeWheelCountDay {
			response.RespErrorWithMsg(c, code.NotEnough, "当天抽奖次数已达上限")
			return
		}
	}

	var lottoPrize = new(cep_ent.LottoPrize)
	// 有抽奖次数，开始抽奖
	if err = db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
		var (
			lottoPrizeID int64
			lottoStatus  = lottorecord.StatusNotGrant
		)
		lottoPrize = handlers.PrizeWheel(_lotto.TotalWeight, _lotto.Edges.LottoPrizes)
		result := lottorecord.ResultWinning
		if lottoPrize == nil {
			// 未中奖
			result = lottorecord.ResultLosing
			lottoPrizeID = 0
		} else {
			// 中奖了
			lottoPrizeID = lottoPrize.ID
			if lottoPrize.Type == lottoprize.TypeGetCep {
				lottoStatus = lottorecord.StatusGranted
				// 如果奖品类型是 get_cep，那就直接发放
				if _, err = logic.GenBillAndUpdateWallet(c, &logic.GenBillAndUpdateWalletOptions{
					Tx:           tx,
					TargetUserID: userID,
					SourceUserID: types.GenesisUserID,
					SymbolID:     types.CepSymbolID,
					Amount:       lottoPrize.CepAmount,
					Type:         enums.BillTypeLotto,
					Way:          enums.BillWayLottoPrize,
				}); err != nil {
					logrus.Errorf("lotto prize wheel gen bill failed, err: %v", err)
					return err
				}
			}

			// 构建推送消息
			msg := new(types.WSPrizeWheelMsg)
			if lottoUserCount.Edges.User != nil {
				msg.Phone = lottoUserCount.Edges.User.Phone
			}
			msg.PrizeName = lottoPrize.Name

			msgBytes, err := json.Marshal(msg)
			if err != nil {
				logrus.Errorf("json marshal prize msg failed, err: %v", err)
				return err
			}

			// 推送广播消息
			if err = myredis.Client.Publish(c, myredis.SubscribeChannelLottoPrizeWheel, msgBytes).Err(); err != nil {
				// 推送消息失败不影响主流程
				logrus.Errorf("redis publish prize msg failed, err: %v", err)
			}
		}
		// 用户在这个活动的抽奖次数减一
		newLottoUserCount, err := tx.LottoUserCount.UpdateOne(lottoUserCount).AddRemainLottoCount(-1).Save(c)
		if err != nil {
			logrus.Errorf("db update lotto user count failed, err: %v", err)
			return err
		}
		// 添加抽奖记录
		if err = tx.LottoRecord.Create().
			SetUserID(userID).
			SetLottoID(req.LottoId).
			SetResult(result).
			SetLottoPrizeID(lottoPrizeID).
			SetStatus(lottoStatus).
			SetRemainLottoCount(newLottoUserCount.RemainLottoCount).
			Exec(c); err != nil {
			logrus.Errorf("db create lotto record failed, err: %v", err)
			return err
		}
		return nil
	}); err != nil {
		response.RespErrorWithMsg(c, code.ServerErrDB, "抽奖失败，请稍后再试")
		return
	}

	// 未中奖返回空对象
	if lottoPrize == nil {
		lottoPrize = &cep_ent.LottoPrize{}
	}

	response.RespSuccess(c, lottoPrize)
}

func ListPrizeRecords(c *gin.Context) {
	// 用户凭证拿到 user_id
	userID, err := getUserId(c)
	if err != nil {
		return
	}

	var req types.ListPrizeRecords
	if err = c.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(c, err)
		return
	}
	logrus.Infof("user: %d, req: %+v", userID, req)

	query := db.DB.LottoRecord.Query().
		Where(lottorecord.UserIDEQ(userID), lottorecord.LottoIDEQ(req.LottoId)).
		WithLottoPrize(func(prizeQuery *cep_ent.LottoPrizeQuery) {
			prizeQuery.Where(lottoprize.DeletedAtEQ(common.ZeroTime))
		}).Order(cep_ent.Desc(lottorecord.FieldCreatedAt))

	total, err := query.Count(c)
	if err != nil {
		logrus.Errorf("user(userID: %d) failed to count lotto(lottoID: %d) record, err: %v", userID, req.LottoId, err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	lottoRecords, err := query.Offset(req.PageSize * (req.PageIndex - 1)).
		Limit(req.PageSize).
		All(c)
	if err != nil {
		logrus.Errorf("user(userID: %d) failed to query lotto(lottoID: %d) record, err: %v", userID, req.LottoId, err)
		response.RespError(c, code.ServerErrDB)
		return
	}

	var resp response.PaginateResp
	resp.List = lottoRecords
	resp.PageIndex = req.PageIndex
	resp.PageSize = req.PageSize
	resp.Total = int64(total)

	response.RespSuccess(c, resp)
}
