package cron

import "github.com/robfig/cron/v3"

func NewCron() *cron.Cron {
	c := cron.New()
	// 添加监控设备状态的任务
	if _, err := c.AddFunc("@every 3m", DeviceHeartBeatForStatus); err != nil {
		panic(err)
	}
	//定时任务， 检查
	if _, err := c.AddFunc("@every 1m", CheckFrozenMoney); err != nil {
		panic(err)
	}
	// 添加余额监控的任务
	if _, err := c.AddFunc("@every 1m", BalanceMonitor); err != nil {
		panic(err)
	}
	// 添加更新冻结资金的任务
	if _, err := c.AddFunc("@every 1m", UpdateUserFreezeMoney); err != nil {
		panic(err)
	}
	// 包时任务订单临期检查任务 - 每十分钟检查一次
	//if _, err := c.AddFunc("@every 10m", cron2.MissionDeadlineCheck); err != nil {
	//	panic(err)
	//}
	// 包时任务订单临期检查任务（按小时包时类型） - 每一分钟检查一次
	if _, err := c.AddFunc("@every 1m", MissionDeadlineCheckHour); err != nil {
		panic(err)
	}
	// 包时任务订单临期检查任务（按天包时类型） - 每十分钟检查一次
	if _, err := c.AddFunc("@every 10m", MissionDeadlineCheckDay); err != nil {
		panic(err)
	}
	// 包时任务订单临期检查任务（按周和按月包时类型） - 每一小时检查一次
	if _, err := c.AddFunc("@every 60m", MissionDeadlineCheckWeekAndMonth); err != nil {
		panic(err)
	}
	// 任务疑似卡住检查
	if _, err := c.AddFunc("@every 3m", MonitorDeadMissions); err != nil {
		panic(err)
	}
	// 任务售罄检查
	//if _, err := c.AddFunc("@every 30s", PriceSoldOut); err != nil {
	//	panic(err)
	//}
	if _, err := c.AddFunc("@every 10s", PriceSoldOut); err != nil {
		panic(err)
	}
	// 定时结算任务订单 - 每天中午十二点结算
	//if _, err := c.AddFunc("0 12 * * *", MissionOrderSettled); err != nil {
	//	panic(err)
	//}

	// 模拟投票
	if _, err := c.AddFunc("@every 5m", MockVote); err != nil {
		panic(err)
	}

	// 运行时间过长的应用预警 - 6小时、12小时、24小时
	if _, err := c.AddFunc("@every 10m", SupplyingMissionTooLongWarning); err != nil {
		panic(err)
	}
	// 同步奖品缓存 - 每三分钟一次（为了推送假中奖数据）
	if _, err := c.AddFunc("@every 3m", SyncLottoPrizeCache); err != nil {
		panic(err)
	}

	return c
}
