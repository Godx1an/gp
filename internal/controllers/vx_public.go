package controllers

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/gin-gonic/gin"
	redis2 "github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/serializer/code"
	"github.com/stark-sim/serializer/response"
	"io"
	"net/http"
	"sort"
	"time"
	"vpay/internal/handlers"
	"vpay/internal/myredis"
	"vpay/internal/types"
)

// VxCheckpoint 微信公众平台进行接口校验发送 get 请求
func VxCheckpoint(context *gin.Context) {
	signature := context.Query("signature")
	echostr := context.Query("echostr")
	timestamp := context.Query("timestamp")
	nonce := context.Query("nonce")
	token := "123"
	var ary []string
	ary = append(ary, token, timestamp, nonce)
	sort.Strings(ary)
	str := ary[0] + ary[1] + ary[2]
	hasher := sha1.New()
	hasher.Write([]byte(str))
	hash := hasher.Sum(nil)
	hashString := hex.EncodeToString(hash)
	if hashString == signature {
		context.String(200, echostr)
		return
	} else {
		response.RespErrorWithMsg(context, code.ServerErr, "验证失败")
	}
}

// VxReply 微信公众平台自动回复
func VxReply(c *gin.Context) {
	var textMsg types.MixMessage
	err := c.ShouldBindXML(&textMsg)
	if err != nil {
		fmt.Printf("XML数据包解析失败: %v\n", err)
		return
	}
	token, err := myredis.Client.Get(c, types.RedisVxPlatformAccessToken).Result()
	if err == redis2.Nil {
		token, err = handlers.VxPlatformGetAccessToken(c)
	}

	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "获取access_token失败")
		return
	}

	if textMsg.Event == "SCAN" {
		if textMsg.Ticket != "" {
			logrus.Info("扫码登录ing")
			_, err = myredis.Client.Set(c, textMsg.Ticket, textMsg.FromUserName, myredis.CacheExpire).Result()
			if err != nil {
				logrus.Error(err)
				return
			}
			return
		}
	} else {
		fmt.Printf("收到用户%v\n的消息\n", textMsg.FromUserName)
		reTextMsg := &types.MixMessage{}
		reTextMsg.FromUserName = textMsg.ToUserName
		reTextMsg.ToUserName = textMsg.FromUserName
		reTextMsg.CreateTime = time.Now().Unix()
		reTextMsg.MsgType = "text"
		if types.General1[textMsg.Content] || textMsg.EventKey == "F1S4" { //复用小助手
			reTextMsg.Content = types.ReGeneral1
			go func() {
				time.Sleep(time.Millisecond * 200)
				ExtraMessage(token, textMsg.FromUserName, types.AssistantImageID)
			}()
		} else if types.General2[textMsg.Content] || textMsg.EventKey == "F1S3" { //复用商务合作
			reTextMsg.Content = types.ReGeneral2
			go func() {
				time.Sleep(time.Millisecond * 200)
				ExtraMessage(token, textMsg.FromUserName, types.BussinessImageID)
			}()
			//	以下为回复数字事件
		} else if textMsg.Content == "1" {
			reTextMsg.Content = types.Reply1
			go func() {
				time.Sleep(time.Millisecond * 200)
				ExtraMessage(token, textMsg.FromUserName, types.BussinessImageID)
			}()
		} else if textMsg.Content == "2" || types.LoginMsg[textMsg.Content] {
			reTextMsg.Content = types.Reply2
		} else if textMsg.Content == "3" {
			reTextMsg.Content = types.Reply3
			go func() {
				time.Sleep(time.Millisecond * 200)
				ExtraMessage(token, textMsg.FromUserName, types.BussinessImageID)
			}()
		} else if textMsg.Content == "4" {
			reTextMsg.Content = types.Reply4
			//	以下为点击菜单事件
		} else if textMsg.EventKey == "F1S1" {
			reTextMsg.Content = types.Hashrate1
			//go func() {
			//	time.Sleep(time.Millisecond * 200)
			//	ExtraMessage(token, textMsg.FromUserName, bussinessImageID)
			//}()
		} else if textMsg.EventKey == "F1S2" {
			reTextMsg.Content = types.Hashrate2
		} else if textMsg.EventKey == "F2S1" {
			reTextMsg.MsgType = "image"
			reTextMsg.Image.MediaID = types.Pic1
		} else if textMsg.EventKey == "F2S2" {
			reTextMsg.MsgType = "image"
			reTextMsg.Image.MediaID = types.Pic2
		} else if textMsg.EventKey == "F3S1" {
			reTextMsg.Content = types.Community1
			go func() {
				time.Sleep(time.Millisecond * 200)
				ExtraMessage(token, textMsg.FromUserName, types.CommunityImageID)
			}()
		} else if textMsg.EventKey == "F3S2" {
			reTextMsg.Content = types.Community2
			go func() {
				time.Sleep(time.Millisecond * 200)
				ExtraMessage(token, textMsg.FromUserName, types.CommunityImageID)
			}()
		} else if textMsg.EventKey == "F3S3" {
			reTextMsg.Content = types.Community3
			go func() {
				time.Sleep(time.Millisecond * 200)
				ExtraMessage(token, textMsg.FromUserName, types.CommunityImageID2)
			}()
		} else if textMsg.EventKey == "F3S4" {
			reTextMsg.Content = types.Community4
			ExtraMessage(token, textMsg.FromUserName, types.CommunityImageID)
		} else if textMsg.Event == "subscribe" {
			reTextMsg.Content = types.SubscribeReply
		} else {
			return
		}
		msg, err := xml.Marshal(reTextMsg)
		if err != nil {
			fmt.Printf("将对象进行XML编码出错: %v", err)
			return
		}
		c.String(http.StatusOK, string(msg))
	}
}

func ExtraMessage(accessToken string, toUser string, imageId string) {
	reTextMsg := &types.MixMessage{}
	reTextMsg.ToUser = toUser
	reTextMsg.MsgType = "image"
	reTextMsg.Image.MediaID = imageId
	body, err := json.Marshal(reTextMsg)
	if err != nil {
		fmt.Printf("将对象进行JSON编码出错: %v\n", err)
		return
	}
	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/message/custom/send?access_token=%v",
		accessToken)
	//fmt.Println(url)
	resp, err := http.Post(url, "text/json", bytes.NewBuffer(body))
	if err != nil {
		fmt.Printf("err:%v\n", err)
		return
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		//fmt.Println(body)
		var ret = new(types.HttpResopnse)
		myError := json.Unmarshal(body, ret)
		if myError != nil {
			fmt.Printf("error:%v\n", myError)
		} else {
			fmt.Printf("发消息接口响应：%v\n", ret)
		}
	}
}

// GetQr 获取公众号二维码
func GetQr(c *gin.Context) {
	token, err := myredis.Client.Get(c, types.RedisVxPlatformAccessToken).Result()
	if err == redis2.Nil {
		token, err = handlers.VxPlatformGetAccessToken(c)
	}

	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "获取access_token失败")
		return
	}
	url := "https://api.weixin.qq.com/cgi-bin/qrcode/create?access_token=" + token
	req := types.GetQrReq{
		ExpireSeconds: 5 * 60,
		ActionName:    "QR_STR_SCENE",
		ActionInfo: struct {
			Scene struct {
				SceneStr string `json:"scene_str"`
			} `json:"scene"`
		}{
			Scene: struct {
				SceneStr string `json:"scene_str"`
			}{
				SceneStr: "123123",
			},
		},
	}
	marshal, err := json.Marshal(req)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "序列化失败")
		return
	}
	_resp, err := http.Post(url, "application/json", bytes.NewBuffer(marshal))
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "请求获取二维码失败")
		return
	}
	var responseData []byte
	var resp types.GetQrResp
	responseData, _ = io.ReadAll(_resp.Body)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "读取请求体失败")
		return
	}
	err = json.Unmarshal(responseData, &resp)
	if err != nil {
		response.RespErrorWithMsg(c, code.ServerErr, "反序列化失败")
		return
	}

	// 兜底逻辑，如果走到这里还是没有 qr code，那么强制删除 微信给的 access_token，使得这一次失败，但是下一次接口很可能就恢复了
	if resp.Url == "" {
		if err = myredis.Client.Del(c, types.RedisVxPlatformAccessToken).Err(); err != nil {
			logrus.Warnf("try to force del wechat access_token in redis err: %v", err)
		}
	}
	response.RespSuccess(c, resp)
}

func IsTicketExist(c *gin.Context) {
	ticket := c.Query("ticket")
	_, err := myredis.Client.Get(c, ticket).Result()
	var resp types.IsExistResp
	if err == redis2.Nil {
		resp.IsExist = false
		response.RespSuccess(c, resp)
		return
	}
	resp.IsExist = true
	response.RespSuccess(c, resp)
}
