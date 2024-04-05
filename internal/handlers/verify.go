package handlers

import (
	"errors"
	"graduation_project/internal/common"
)

func VerifyPhone(phone string) error {
	l1 := len([]rune(phone))
	if l1 != 11 {
		return errors.New("请填写正确的电话号码")
	}
	return nil
}

func VerifyPwd(pwd string) error {
	flag := common.ValidatePassword(pwd)
	if !flag {
		return errors.New("密码不符合要求")
	}
	return nil
}

func VerifyNickName(nickname string) error {
	if nickname == "" {
		return errors.New("昵称不能为空")
	}
	return nil
}
