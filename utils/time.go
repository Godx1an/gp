package utils

import "time"

var ZeroTime = time.Time{}

func Time2Int(now time.Time) int32 {
	year, month, day := now.Date()
	month += 10
	var res int32
	res = int32(year)*10000 + int32(month)*100 + int32(day)
	return res
}
