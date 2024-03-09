package cache

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"time"
	"vpay/internal/myredis"
)

const RedisLockExpiredTime = time.Minute * 5

var ctx = context.Background()

// GetLock 获取锁
func GetLock(rKey string) error {
	reply, err := myredis.Client.SetNX(ctx, rKey, "", RedisLockExpiredTime).Result()
	if err != nil {
		logrus.Errorf("redis set not exist key(%s) failed, err: %v", rKey, err)
		return err
	}
	if !reply {
		logrus.Warningf("redis set not exist key(%s) reply false", rKey)
		return fmt.Errorf("redis set not exist key(%s) reply false", rKey)
	}
	return nil
}

// FreeLock 释放锁
func FreeLock(rKey string) {
	if err := myredis.Client.Del(ctx, rKey).Err(); err != nil {
		logrus.Errorf("redis del key(%s) failed, err: %v", rKey, err)
		return
	}
}
