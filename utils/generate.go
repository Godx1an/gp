package utils

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/bwmarrin/snowflake"
	"github.com/sirupsen/logrus"
	"math/rand"
	mathRand "math/rand"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	PRIME1 = 3         // 与字符集长度 62 互质
	PRIME2 = 5         // 与邀请码长度 6 互质
	SALT   = 123456789 // 随意取一个数值
)

const (
	RandStringBase = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var AlphanumericSet = []rune{
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
	'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
	'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
}

func GenerateSign(body interface{}, key string) string {
	values := make(map[string]string)

	// 利用反射获取字段值和字段名称
	reqValue := reflect.ValueOf(body)
	reqType := reqValue.Type()

	for i := 0; i < reqValue.NumField(); i++ {
		field := reqValue.Field(i)
		fieldType := reqType.Field(i)

		fieldValue := ""

		switch field.Kind() {
		case reflect.String:
			fieldValue = field.String()
		case reflect.Int:
			fieldValue = fmt.Sprintf("%d", field.Int())
		}

		fieldName := fieldType.Tag.Get("xml")

		// 排除空值字段
		if fieldValue != "" && fieldName != "sign" {
			values[fieldName] = fieldValue
		}
	}

	// 对字段按照字段名的ASCII码从小到大排序
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 按照排序后的字段顺序拼接字符串
	var result []string
	for _, k := range keys {
		result = append(result, fmt.Sprintf("%s=%s", k, values[k]))
	}

	st1 := strings.Join(result, "&")
	st1 = st1 + "&key=" + key
	fmt.Println(st1)
	// 计算MD5签名
	data := []byte(st1)
	has := md5.Sum(data)
	md5str := fmt.Sprintf("%x", has)

	return strings.ToUpper(md5str)
}

func GenerateRandomString() string {
	rand.Seed(time.Now().UnixNano())
	const orderNumberLength = 5
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	// 生成随机字符串
	randString := make([]rune, orderNumberLength)
	for i := 0; i < orderNumberLength; i++ {
		randString[i] = letterRunes[rand.Intn(len(letterRunes))]
	}

	// 生成时间戳，精确到毫秒
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	// 组合随机字符串和时间戳生成订单号
	orderNumber := fmt.Sprintf("%s%d", string(randString), timestamp)

	return orderNumber
}

func GenerateRandomStringWithLength(length int) string {
	// 用雪花生成 uid
	uid := uint64(GenSnowflakeID())

	// 放大 + 加盐
	uid = uid*PRIME1 + SALT

	var code []rune
	slIdx := make([]byte, length)

	// 扩散
	for i := 0; i < length; i++ {
		slIdx[i] = byte(uid % uint64(len(AlphanumericSet)))                   // 获取 62 进制的每一位值
		slIdx[i] = (slIdx[i] + byte(i)*slIdx[0]) % byte(len(AlphanumericSet)) // 其他位与个位加和再取余（让个位的变化影响到所有位）
		uid = uid / uint64(len(AlphanumericSet))                              // 相当于右移一位（62进制）
	}

	// 混淆
	for i := 0; i < length; i++ {
		idx := (byte(i) * PRIME2) % byte(length)
		code = append(code, AlphanumericSet[slIdx[idx]])
	}
	return string(code)
}

// GetInvCodeByUIDUniqueNew 获取指定长度的邀请码
func GetInvCodeByUIDUniqueNew(uid uint64, l int) string {
	// 放大 + 加盐
	uid = uid*PRIME1 + SALT

	var code []rune
	slIdx := make([]byte, l)

	// 扩散
	for i := 0; i < l; i++ {
		slIdx[i] = byte(uid % uint64(len(AlphanumericSet)))                   // 获取 62 进制的每一位值
		slIdx[i] = (slIdx[i] + byte(i)*slIdx[0]) % byte(len(AlphanumericSet)) // 其他位与个位加和再取余（让个位的变化影响到所有位）
		uid = uid / uint64(len(AlphanumericSet))                              // 相当于右移一位（62进制）
	}

	// 混淆
	for i := 0; i < l; i++ {
		idx := (byte(i) * PRIME2) % byte(l)
		code = append(code, AlphanumericSet[slIdx[idx]])
	}
	return string(code)
}

// GenHexString 参数为密钥长度
func GenHexString(len int) string {
	// 生成随机字节序列
	key := make([]byte, len)
	_, err = rand.Read(key)
	if err != nil {
		logrus.Errorf("生成密钥时发生错误：%v", err)
		return ""
	}

	// 将随机字节序列转换为十六进制字符串
	encodedKey := hex.EncodeToString(key)

	return encodedKey
}

func GenSnowflakeInt64() int64 {
	if node == nil {
		node, err = snowflake.NewNode(rand.Int63n(1024))
	}
	// Generate a snowflake ID.
	id := node.Generate()
	return id.Int64()
}

func GenerateRandomUname() string {
	mathRand.New(mathRand.NewSource(time.Now().UnixNano()))
	const orderNumberLength = 6
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	// 生成随机字符串
	randString := make([]rune, orderNumberLength)
	for i := 0; i < orderNumberLength; i++ {
		randString[i] = letterRunes[mathRand.Intn(len(letterRunes))]
	}

	return string(randString)
}

func GenerateRandomPwd() string {
	mathRand.New(mathRand.NewSource(time.Now().UnixNano()))
	const orderNumberLength = 10
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	// 生成随机字符串
	randString := make([]rune, orderNumberLength)
	for i := 0; i < orderNumberLength; i++ {
		randString[i] = letterRunes[mathRand.Intn(len(letterRunes))]
	}

	return string(randString)
}
