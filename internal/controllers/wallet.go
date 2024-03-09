package controllers

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/symbol"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/user"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/wallet"
	"github.com/stark-sim/serializer"
	"github.com/stark-sim/serializer/response"
	"net/http"
	"regexp"
	"strconv"
	"sync"
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

// GetOnesWallets 获取个人的钱包情况
func GetOnesWallets(c *gin.Context) {
	userID, err := common.GetUserId(c)
	if err != nil {
		response.DynamicRespErr(c, err)
		return
	}
	wallets, err := handlers.GetOrGenerateWalletsByUserID(c, &handlers.GetWalletsByUserIDOptions{
		Tx:     nil,
		UserID: userID,
	})
	if err != nil {
		response.DynamicRespErr(c, err)
	}
	response.RespSuccess(c, serializer.FrontendSerialize(wallets))
}

var upgrader *websocket.Upgrader

func init() {
	upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
}

func FetchWalletsStatus(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logrus.Errorf("new ws conn err: %v", err)
		return
	}
	// 第一次最多等 10 s
	if err = conn.SetReadDeadline(time.Now().Add(120 * time.Second)); err != nil {
		logrus.Errorf("set ws read deadline err: %v", err)
		return
	}

	defer func(conn *websocket.Conn) {
		if err := conn.Close(); err != nil {
			logrus.Errorf("conn defer close err: %v", err)
			return
		}
	}(conn)

	// 控制关闭信道只能被调用一次
	stopCh := make(chan struct{})

	// 考虑到并发情况所以管道容量设置为 10 （其实具体数额可以根据实际运行情况来定， uber 提供的 go 开发规范指出 管道容量最好为 0 或 1）
	connRespMsg := make(chan []byte, 10)
	defer close(connRespMsg)

	var userID int64
	// 当输入协程得到 userID 后，其他协程才可以开始
	hasUserIDChan := make(chan struct{})
	neverHaveUserID := true

	// 重置心跳
	go func(stopCh chan struct{}) {
		defer func() {
			logrus.Info("重置心跳结束")
			if v := recover(); v != nil {
				logrus.Errorf("ws reset heart beat panic: %v", v)
				select {
				case <-stopCh:
					return
				default:
					close(stopCh)
					return
				}
			}
		}()
		for {
			select {
			case <-stopCh:
				return
			default:
				// 第一步先读前端发来的 Authorization
				_, p, err := conn.ReadMessage()
				if err != nil {
					RespWsErr(connRespMsg, err)
					close(stopCh)
					return
				}
				var req struct {
					Authorization string `json:"authorization"`
				}
				if err = json.Unmarshal(p, &req); err != nil {
					RespWsErr(connRespMsg, err)
					close(stopCh)
					return
				}
				userID, err, _ = middleware.TokenToUserID(req.Authorization)
				if err != nil {
					RespWsErr(connRespMsg, err)
					close(stopCh)
					return
				}
				// 前端心跳就 reset timer 延长 60 s
				if err = conn.SetReadDeadline(time.Now().Add(120 * time.Second)); err != nil {
					logrus.Errorf("ws reset timer err: %v", err)
					return
				}

				// 需要接收前端请求的这个协程完成后，其他协程才可以启动
				if neverHaveUserID {
					hasUserIDChan <- struct{}{}
					neverHaveUserID = false
				}
			}
		}
	}(stopCh)

	// 需要在具备了 userID 后才可以工作的协程需要在获得同步量后再开始
	select {
	case <-stopCh:
		time.Sleep(3 * time.Second)
		return
	case <-hasUserIDChan:
		// 请求初始状态，获取用户所有钱包信息 (通过管道接收信号量来判断是否可以开始获取信息)
		go func(stopCh chan struct{}) {
			for {
				select {
				case <-stopCh:
					return
				default:
					defer func() {
						logrus.Info("初始化请求结束")
						if v := recover(); v != nil {
							logrus.Errorf("ws get wallet info panic: %v", v)
							select {
							case <-stopCh:
								return
							default:
								close(stopCh)
								return
							}
						}
					}()
					// 找出用户的账户
					var _user *cep_ent.User
					if txErr := db_utils.WithTx(c, nil, func(tx *cep_ent.Tx) error {
						_user, err = tx.User.Query().
							Where(user.DeletedAt(utils.ZeroTime), user.ID(userID)).
							WithWallets(func(walletQuery *cep_ent.WalletQuery) {
								walletQuery.Where(wallet.DeletedAt(utils.ZeroTime)).WithSymbol()
							}).
							First(c)
						if err != nil {
							logrus.Error(err)
							return err
						}

						_symbol, err := tx.Symbol.Query().Where(symbol.DeletedAt(utils.ZeroTime)).All(c)
						if err != nil {
							return err
						}
						if _user.Edges.Wallets == nil || len(_user.Edges.Wallets) < len(_symbol) {

							// DONE: 修改成 wallet 的类型
							_user.Edges.Wallets, err = handlers.CompletionWallet(c, &types.CompletionWalletOption{
								UserID: userID,
								Tx:     tx,
							})
							if err != nil {
								return err
							}
						}
						return nil
					}); txErr != nil {
						RespWsErr(connRespMsg, txErr)
						close(stopCh)
						return
					}

					// 找出用户所有冻结的金额
					userFreezeKeys, err := myredis.Client.Keys(c, fmt.Sprintf(types.RedisKeyUserFreezeWalletsMoney, userID, "*", "*")).Result()
					if err != nil && err != redis.Nil {
						RespWsErr(connRespMsg, err)
						close(stopCh)
						return
					}
					// 计算实际可用剩余余额
					//sumFreezeAmount := make(map[int64]int64) // map[币种]金额
					var sumFreezeAmountIndex sync.Map
					for _, userFreezeKey := range userFreezeKeys {
						// 获取这一单的冻结量
						itemFreezeCep, err := myredis.Client.Get(c, userFreezeKey).Int64()
						if err != nil && err != redis.Nil {
							RespWsErr(connRespMsg, err)
							close(stopCh)
							return
						}
						re := regexp.MustCompile(fmt.Sprintf(types.RedisKeyUserFreezeWalletsMoney, `(\d{19})`, `(\d{19})`, `(\d+)`))
						matches := re.FindAllStringSubmatch(userFreezeKey, -1)
						var symbolID int64
						for _, match := range matches {
							// 当前这笔冻结单是什么币种
							symbolID, err = strconv.ParseInt(match[3], 10, 64)
							if err != nil {
								RespWsErr(connRespMsg, err)
								close(stopCh)
								return
							}
							// 再从用户的钱包中找到对应钱包
							for _, _wallet := range _user.Edges.Wallets {
								if symbolID == _wallet.SymbolID {
									// sumFreezeAmount[_wallet.SymbolID] += itemFreezeCep
									value, _ := sumFreezeAmountIndex.Load(_wallet.SymbolID)
									var cep int64
									if value == nil {
										cep = 0
									} else {
										cep = value.(int64)
									}
									sumFreezeAmountIndex.Store(_wallet.SymbolID, cep+itemFreezeCep)
									break
								}
							}
						}
					}
					var walletResp types.WalletResp
					for _, _wallet := range _user.Edges.Wallets {
						value, _ := sumFreezeAmountIndex.Load(_wallet.SymbolID)
						var cep int64
						if value == nil {
							cep = 0
						} else {
							cep = value.(int64)
						}
						walletResp.Wallets = append(walletResp.Wallets, &types.WrapWallet{
							Wallet: _wallet,
							// RealAmount: _wallet.Amount - sumFreezeAmount[_wallet.SymbolID],
							RealAmount: _wallet.Amount - cep,
						})
					}
					walletData := serializer.FrontendSerialize(walletResp.Wallets)
					// 推给前端
					respData, err := json.Marshal(walletData)
					connRespMsg <- respData

					// 每次推送后休息一下
					time.Sleep(30 * time.Second)
				}
			}
		}(stopCh)

		// 附加监听用户余额是否不足预警
		go func(stopCh chan struct{}) {
			defer func() {
				logrus.Info("预警监听结束")
				if v := recover(); v != nil {
					logrus.Errorf("ws balance monitor panic: %v", v)
					select {
					case <-stopCh:
						return
					default:
						close(stopCh)
						return
					}
				}
			}()
			for {
				select {
				case <-stopCh:
					return
				default:
					// 此操作为阻塞操作
					msg, err := myredis.UserMissionSub.ReceiveMessage(c)
					if err != nil {
						RespWsErr(connRespMsg, err)
						continue
					}
					var publishWarnUserBalanceData types.PublishWarnUserBalanceData
					if err = json.Unmarshal([]byte(msg.Payload), &publishWarnUserBalanceData); err != nil {
						RespWsErr(connRespMsg, err)
						continue
					}

					// 当前用户出现余额预警
					if publishWarnUserBalanceData.UserID == userID {

						resData, _ := json.Marshal(map[string]interface{}{
							"type":      "balance",
							"msg":       "用户余额即将不足以支撑计时任务运行",
							"symbol_id": publishWarnUserBalanceData.SymbolID,
						})
						connRespMsg <- resData
					}
				}
			}
		}(stopCh)

		// 实时监听 wallet 是否变动
		go func(stopCh chan struct{}) {
			defer func() {
				logrus.Info("实时监听wallet结束")
				if v := recover(); v != nil {
					logrus.Errorf("ws wallet monitor panic: %v", v)
					select {
					case <-stopCh:
						return
					default:
						close(stopCh)
						return
					}
				}
			}()
			for {
				select {
				case <-stopCh:
					return
				default:
					// 订阅钱包余额不足的 redis 广播
					// 接收信息并过滤
					msg, err := myredis.UserWalletSub.ReceiveMessage(c)
					if err != nil {
						logrus.Error(err)
						RespWsErr(connRespMsg, err)
						// 信息错误，也可以继续监听
						continue
					}
					// 收到广播信息，解析信息内容
					var publishWalletData types.WalletPublish
					if err = json.Unmarshal([]byte(msg.Payload), &publishWalletData); err != nil {
						logrus.Error(err)
						RespWsErr(connRespMsg, err)
						continue
					}
					// 如果广播信息就是说当前 userID 的事
					if publishWalletData.UserID == userID {
						time.Sleep(100 * time.Millisecond)
						// 找到冻结金额
						_userFreezeKeys, err := myredis.Client.Keys(c, fmt.Sprintf(types.RedisKeyUserFreezeWalletsMoney, publishWalletData.UserID, "*", publishWalletData.SymbolID)).Result()
						if err != nil {
							RespWsErr(connRespMsg, err)
							continue
						}

						//_sumFreezeAmount := make(map[int64]int64) // map[币种]金额
						var _sumFreezeAmountIndex sync.Map
						for _, userFreezeKey := range _userFreezeKeys {
							itemFreezeCep, err := myredis.Client.Get(c, userFreezeKey).Int64()
							if err != nil {
								RespWsErr(connRespMsg, err)
								continue
							}
							// _sumFreezeAmount[publishWalletData.SymbolID] = _sumFreezeAmount[publishWalletData.SymbolID] + itemFreezeCep
							value, _ := _sumFreezeAmountIndex.Load(publishWalletData.SymbolID)
							var cep int64
							if value == nil {
								cep = 0
							} else {
								cep = value.(int64)
							}
							_sumFreezeAmountIndex.Store(publishWalletData.SymbolID, cep+itemFreezeCep)
						}

						_wallet, err := db.DB.Wallet.Query().Where(wallet.UserID(publishWalletData.UserID), wallet.SymbolID(publishWalletData.SymbolID)).First(c)
						if err != nil {
							RespWsErr(connRespMsg, err)
							close(stopCh)
							return
						}
						value, _ := _sumFreezeAmountIndex.Load(_wallet.SymbolID)
						var cep int64
						if value == nil {
							cep = 0
						} else {
							cep = value.(int64)
						}
						var walletResp types.WalletResp
						walletResp.Wallets = append(walletResp.Wallets, &types.WrapWallet{
							Wallet: _wallet,
							//RealAmount: _wallet.Amount - _sumFreezeAmount[_wallet.SymbolID],
							RealAmount: _wallet.Amount - cep,
						})

						walletData := serializer.FrontendSerialize(walletResp.Wallets)
						// 推给前端
						respData, err := json.Marshal(walletData)
						if err != nil {
							RespWsErr(connRespMsg, err)
							close(stopCh)
						}
						connRespMsg <- respData

					}
				}
			}
		}(stopCh)

		// 对管道 connRespMsg 的内容内容进行发送，避免 ws 产生并发问题
		go func(stopCh chan struct{}) {
			defer func() {
				logrus.Info("管道 connRespMsg 结束")
				if v := recover(); v != nil {
					logrus.Errorf("ws wallet monitor panic: %v", v)
					select {
					case <-stopCh:
						return
					default:
						close(stopCh)
						return
					}
				}
			}()
			for {
				select {
				case <-stopCh:
					return
				case resData := <-connRespMsg:
					if err = conn.WriteMessage(websocket.TextMessage, resData); err != nil {
						logrus.Errorf("send data to ws conn err: %v", err)
						close(stopCh)
						return
					}
				}
			}
		}(stopCh)
	}
	// 所有逻辑都在协程中进行，主协程等待他们结束
	select {
	case <-stopCh:
		// 让子协程运行结束
		time.Sleep(3 * time.Second)
		return
	}
}

type WsErrMsg struct {
	Type string `json:"type"`
	Msg  string `json:"msg"`
}

// RespWsErr 只负责返回数据，不包含影响关闭 ws 链接的操作。chan 关键字本身是指针类型，作为参数时不需要使用指针
func RespWsErr(respConnMsg chan []byte, err error) {
	respData, marshalErr := json.Marshal(WsErrMsg{
		Type: "error",
		Msg:  err.Error(),
	})
	if marshalErr != nil {
		logrus.Errorf("resp ws occure err: %v", marshalErr)
		return
	}
	respConnMsg <- respData
}
