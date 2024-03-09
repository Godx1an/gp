package controllers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/common"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/artwork"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/artworklike"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"time"
	_common "vpay/internal/common"
	"vpay/internal/db"
	"vpay/internal/myredis"
	"vpay/internal/types"
	"vpay/utils"
	"vpay/utils/cache"
	"vpay/utils/db_utils"
)

const (
	RedisKeyLockVoteArtwork = "cephalon:lock:vote:user.id:%d:artwork.id:%d:string"
)

type ArtworkWithVotes struct {
	Artwork *cep_ent.Artwork `json:"artwork"`
	VoteNum int64            `json:"vote_num"`
	Voted   bool             `json:"voted"`
}

type ListArtworksReq struct {
	UserID int64 `form:"user_id,string"`
}

func ListArtworks(ctx *gin.Context) {
	var req ListArtworksReq
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 判断是否在白名单
	isWhite, err := myredis.Client.SIsMember(ctx, types.RedisWhiteVoteUserSet, req.UserID).Result()
	if err != nil {
		logrus.Errorf("query redis to judge white vote user failed, err: %v", err)
		response.RespError(ctx, code.ServerErrCache)
		return
	}

	var resp []ArtworkWithVotes
	if txErr := db_utils.WithClient(nil, func(client *cep_ent.Client) error {

		// 查询所有作品
		artworks, err := client.Artwork.Query().Where(artwork.DeletedAt(common.ZeroTime)).All(ctx)
		if err != nil {
			logrus.Errorf("db query atrworks failed, err: %v", err)
			return err
		}

		// 统计票数
		var voteNum []struct {
			ID    int64 `json:"artwork_id"`
			Count int64 `json:"count"`
		}
		err = client.ArtworkLike.Query().
			GroupBy(artworklike.FieldArtworkID).
			Aggregate(cep_ent.As(cep_ent.Count(), "count")).
			Scan(ctx, &voteNum)
		if err != nil {
			logrus.Errorf("db count artwork votes failed, err: %v", err)
			return err
		}

		// 将票数以 Map 形式统计
		voteNumMap := make(map[int64]int64)
		for _, item := range voteNum {
			voteNumMap[item.ID] = item.Count
		}

		// 查询该用户今日投票列表
		likes, err := db.DB.ArtworkLike.Query().Where(
			artworklike.UserIDEQ(req.UserID),
			artworklike.DeletedAt(common.ZeroTime),
			artworklike.DateEQ(utils.Time2Int(time.Now()))).All(ctx)
		if err != nil {
			logrus.Errorf("db query artwork votes failed, err: %v", err)
			return err
		}

		// 今日投票过的作品以 Map 形式统计
		votedMap := make(map[int64]bool)
		// 只统计非白名单中的用户是否投过票
		if !isWhite {
			for _, item := range likes {
				votedMap[item.ArtworkID] = true
			}
		}

		// 处理返回体
		for _, _artwork := range artworks {
			var num int64
			var artworkWithVotes ArtworkWithVotes
			num, ok := voteNumMap[_artwork.ID]
			if !ok {
				// 查不到票数 默认为 0
				logrus.Warningf("artwork(%d) failed to get votes", _artwork.ID)
				num = 0
			}
			_, voted := votedMap[_artwork.ID]
			if voted {
				artworkWithVotes.Voted = true
			}
			artworkWithVotes.VoteNum = num
			artworkWithVotes.Artwork = _artwork
			resp = append(resp, artworkWithVotes)
		}

		return nil
	}); txErr != nil {
		logrus.Errorf("failed to list artworks, err:%v", txErr)
		response.RespError(ctx, code.ServerErrDB)
		return
	}

	response.RespSuccess(ctx, resp)
}

type VoteArtworkReq struct {
	ArtworkID int64 `form:"artwork_id,string" json:"artwork_id,string" binding:"required"`
}

// VoteArtWork 投票
func VoteArtWork(ctx *gin.Context) {
	userID, err := _common.GetUserId(ctx)
	if err != nil {
		response.RespError(ctx, code.UnLogin)
		return
	}

	location, err := time.LoadLocation("Asia/Shanghai")
	if time.Now().After(time.Date(2024, 01, 17, 0, 0, 0, 0, location)) || time.Now().Before(time.Date(2024, 01, 10, 0, 0, 0, 0, location)) {
		response.RespErrorWithMsg(ctx, code.InvalidParams, "不在投票时间")
		return
	}

	// 判断是否在白名单
	isWhite, err := myredis.Client.SIsMember(ctx, types.RedisWhiteVoteUserSet, userID).Result()
	if err != nil {
		logrus.Errorf("query redis to judge white vote user failed, err: %v", err)
		response.RespError(ctx, code.ServerErrCache)
		return
	}

	var req VoteArtworkReq
	if err := ctx.ShouldBind(&req); err != nil {
		response.RespErrorInvalidParams(ctx, err)
		return
	}
	logrus.Debugf("req: %+v", req)

	// 抢锁 - 处理冻结金额 - 更新缓存
	rKeyCache := fmt.Sprintf(RedisKeyLockVoteArtwork, userID, req.ArtworkID)
	if err := cache.GetLock(rKeyCache); err != nil {
		logrus.Errorf("get redis lock cache key(%s) failed, err: %v", rKeyCache, err)
		return
	}
	// 及时释放
	defer cache.FreeLock(rKeyCache)

	now := time.Now()
	timeTag := utils.Time2Int(now)

	// 投票规则检验
	// 不在白名单的用户 一天只能投三票 对一个作品只能投一票
	if !isWhite {
		// 检查投了多少票
		votedCount, err := db.DB.ArtworkLike.Query().
			Where(artworklike.UserIDEQ(userID), artworklike.DateEQ(timeTag)).
			Count(ctx)
		if err != nil {
			logrus.Errorf("failed to count the votes cast today, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return
		}

		if votedCount >= 3 {
			response.RespErrorWithMsg(ctx, code.NotEnough, "今日投票已达上限")
			return
		}

		// 检查有没有为当前作品投过票
		count, err := db.DB.ArtworkLike.Query().
			Where(artworklike.UserIDEQ(userID), artworklike.ArtworkIDEQ(req.ArtworkID), artworklike.DateEQ(timeTag)).
			Count(ctx)
		if err != nil {
			logrus.Errorf("failed to determine whether the user has voted, err: %v", err)
			response.RespError(ctx, code.ServerErrDB)
			return
		}
		if count > 0 {
			response.RespErrorWithMsg(ctx, code.NotEnough, "今日已为该作品投票！")
			return
		}
	}
	artworkLike, err := db.DB.ArtworkLike.Create().
		SetUserID(userID).
		SetArtworkID(req.ArtworkID).
		SetDate(timeTag).
		Save(ctx)
	if err != nil {
		logrus.Errorf("failed to vote, err: %v", err)
		response.RespError(ctx, code.ServerErrDB)
		return
	}
	response.RespSuccess(ctx, artworkLike)
}
