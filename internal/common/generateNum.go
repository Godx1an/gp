package common

import (
	"context"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	"graduation_project/utils/db_utils"
	"strconv"
	"strings"
	"time"
)

// 传入一个参数i，拼接成一个字符串，返回字符串，字符串长度为11位数，前面是1371570，后面是i
func GeneratePhoneNumber(i int) string {
	prefix := "1371570"

	// Convert the input integer to a string
	iStr := strconv.Itoa(i)

	// Ensure that the resulting string has at least 4 digits (to make up for the remaining 4 positions after the prefix)
	if len(iStr) < 4 {
		iStr = strings.Repeat("0", 4-len(iStr)) + iStr
	}

	// Concatenate the prefix and the padded integer string
	print(prefix + iStr)
	return prefix + iStr
}

func add100users() {
	c := context.Background()
	if err := db_utils.WithTx(c, nil, func(tx *ent_work.Tx) error {
		var UserCreates = make([]*ent_work.UserCreate, 0)
		for i := 3; i < 100; i++ {
			create := tx.User.Create().
				SetPhone(GeneratePhoneNumber(i)).
				SetSchool("东莞理工学院").
				SetPassword("12345q").
				SetNickname("test" + strconv.Itoa(i)).
				SetNextUpdateTime(time.Now())
			UserCreates = append(UserCreates, create)
		}
		if err := tx.User.CreateBulk(UserCreates...).Exec(c); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return
	}
}
