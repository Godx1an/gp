package common

import (
	"fmt"
	"github.com/badoux/checkmail"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
	"graduation_project/configs"
)

const (
	EmailHost     = "smtp.qq.com"
	EmailPort     = 465
	Title         = "体测 平台验证信息"
	MessagePrefix = "验证码有效期为 5 分钟,每个验证码只可使用一次，请尽快进行验证\n验证码为: "
)

// SendMail 发送验证码
func SendMail(targetEmail string, code string) error {

	// 创建连接
	conn := gomail.NewDialer(EmailHost, EmailPort, configs.Conf.Email.Username, configs.Conf.Email.Password)
	// 校验目的邮箱是否可用
	err := checkmail.ValidateFormat(targetEmail)
	if err != nil {
		logrus.Errorf("email address %s is not available, err: %v", targetEmail, err)
		return err
	}

	// 创建邮件信息
	message := gomail.NewMessage()
	message.SetHeader("From", configs.Conf.Email.Username)
	message.SetHeader("To", targetEmail)
	message.SetHeader("Subject", Title)
	message.SetBody("text/html", fmt.Sprintf(MessagePrefix+"%v", code))

	// 发送
	err = conn.DialAndSend(message)
	if err != nil {
		logrus.Errorf("Send message to %d failed! err: %v", err)
		return err
	}

	return nil

}
