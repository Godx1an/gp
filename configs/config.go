package configs

import (
	"context"
	"flag"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
)

const DefaultConfigPath = "./config.yaml"

var Conf = new(Config)

var (
	Ctx    context.Context
	Cancel context.CancelFunc
)

type Config struct {
	DBConfig         `mapstructure:"db"`
	AppConfig        `mapstructure:"app"`
	RedisConfig      `mapstructure:"redis"`
	BaiDuConfig      `mapstructure:"baidu"`
	VxAppConfig      `mapstructure:"vx_app"`
	SwiftPass        `mapstructure:"swift_pass"`
	CallBackConfig   `mapstructure:"call_back"`
	YunPian          `mapstructure:"yun_pian"`
	Email            `mapstructure:"email"`
	SmsTencentConfig `mapstructure:"sms_tencent"`
}

type SmsTencentConfig struct {
	SdkAppId  string `mapstructure:"sdk_app_id"`
	SecretId  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
}

type Email struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type YunPian struct {
	ApiKey string `mapstructure:"api_key"`
}

type SwiftPass struct {
	BusinessID string `mapstructure:"business_id"`
	MchID      string `mapstructure:"mch_id"`
	Key        string `mapstructure:"key"`
}

type CallBackConfig struct {
	Ip   string `mapstructure:"ip"`
	Host string `mapstructure:"host"`
}

type AppConfig struct {
	AppId     string `mapstructure:"app_id"`
	AppSecret string `mapstructure:"app_secret"`
	MchId     string `mapstructure:"mch_id"`
	Key       string `mapstructure:"key"`
}

type VxAppConfig struct {
	YuanHui            `mapstructure:"yuan_hui"`
	CephalonCore       `mapstructure:"cephalon_core"`
	CephalonVxPlatform `mapstructure:"cephalon_vx_platform"`
}
type YuanHui struct {
	AppParameters `mapstructure:"params"`
}
type CephalonCore struct {
	AppParameters `mapstructure:"params"`
}
type CephalonVxPlatform struct {
	AppParameters `mapstructure:"params"`
}

type AppParameters struct {
	AppID     string `mapstructure:"app_id"`
	AppSecret string `mapstructure:"app_secret"`
}

type BaiDuConfig struct {
	AppId     string `mapstructure:"app_id"`
	AppSecret string `mapstructure:"app_secret"`
}

type DBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func init() {
	// 默认配置文件路径
	var configPath string
	flag.StringVar(&configPath, "config", "", "配置文件绝对路径或相对路径")
	flag.Parse()
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	logrus.Printf("===> config path is: %s", configPath)
	// 初始化配置文件
	viper.SetConfigFile(configPath)
	viper.WatchConfig()
	// 观察配置文件变动
	viper.OnConfigChange(func(in fsnotify.Event) {
		logrus.Printf("config file has changed")
		if err := viper.Unmarshal(&Conf); err != nil {
			logrus.Errorf("failed at unmarshal config file after change, err: %v", err)
			panic(fmt.Sprintf("failed at init config: %v", err))
		}
	})
	// 将配置文件读入 viper
	if err := viper.ReadInConfig(); err != nil {
		logrus.Errorf("failed at ReadInConfig, err: %v", err)
		panic(fmt.Sprintf("failed at init config: %v", err))
	}
	// 解析到变量中
	if err := viper.Unmarshal(&Conf); err != nil {
		logrus.Errorf("failed at Unmarshal config file, err: %v", err)
		panic(fmt.Sprintf("failed at init config: %v", err))
	}
	// 如果有环境变量就覆盖，适用于本地开发使用文件，实际运行使用环境变量的场景
	cephalonDBHost := os.Getenv("CEPHALON_DB_HOST")
	if cephalonDBHost != "" {
		Conf.DBConfig.Host = cephalonDBHost
	}
	cephalonDBPassword := os.Getenv("CEPHALON_DB_PASSWORD")
	if cephalonDBPassword != "" {
		Conf.DBConfig.Password = cephalonDBPassword
	}
	cephalonRedisHost := os.Getenv("CEPHALON_REDIS_HOST")
	if cephalonRedisHost != "" {
		Conf.RedisConfig.Host = cephalonRedisHost
	}
	cephalonRedisPassword := os.Getenv("CEPHALON_REDIS_PASSWORD")
	if cephalonRedisPassword != "" {
		Conf.RedisConfig.Password = cephalonRedisPassword
	}
	cephalonSmsTencentSecretId := os.Getenv("SECRET_ID")
	if cephalonSmsTencentSecretId != "" {
		Conf.SmsTencentConfig.SecretId = cephalonSmsTencentSecretId
	}
	cephalonSmsTencentSecretKey := os.Getenv("SECRET_KEY")
	if cephalonSmsTencentSecretKey != "" {
		Conf.SmsTencentConfig.SecretKey = cephalonSmsTencentSecretKey
	}
	// 返回 nil 或错误
	logrus.Infoln("global config init success...")
	logrus.Infof("VxApp's all config: %+v", Conf.VxAppConfig)
	Ctx, Cancel = context.WithCancel(context.Background())
	logrus.Infof("%+v", Conf)
}
