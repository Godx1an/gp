package utils

import (
	"github.com/go-playground/validator/v10"
	"reflect"
	"regexp"
)

//var (
//	validate = validator.New()
//	_        = validate.RegisterValidation("phone", verifyPhone)
//)

// ValidatePhone 验证手机号
func ValidatePhone(fl validator.FieldLevel) bool {
	regular := "^((13[0-9])|(14[5,7])|(15[0-3,5-9])|(17[0,3,5-8])|(18[0-9])|166|198|199|(147))\\d{8}$"
	reg := regexp.MustCompile(regular)
	field := fl.Field()
	switch field.Kind() {
	case reflect.String:
		return reg.MatchString(field.String())
	default:
		return false
	}
}
