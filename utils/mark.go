package utils

import "fmt"

// MaskPhone 手机号加密
func MaskPhone(phone string) string {
	if len(phone) <= 10 {
		return phone
	}
	phone = phone[:3] + "*******"
	markedPhone := fmt.Sprintf("用户 " + phone)
	return markedPhone
}
