package types

const RegisterCodeLength = 6

const (
	RedisEmailCodeExpireTime = 5                         // 验证码有效期
	RedisEmailCode           = "gp:code:email:%s:string" // 1. 发送验证码目的  2. 邮箱
	RedisItem                = "gp:item:%s:string"
)
