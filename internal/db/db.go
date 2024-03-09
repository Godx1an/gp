package db

import (
	"database/sql"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"fmt"
	"github.com/Godx1an/gp_ent/pkg/ent_work"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
	"graduation_project/configs"
	"time"
)

var url = ""
var DB *ent_work.Client

func init() {
	dbConf := configs.Conf.DBConfig
	url = fmt.Sprintf("postgres://%s:%s@%s:%v/%s?sslmode=disable&TimeZone=Asia/Shanghai", dbConf.Username, dbConf.Password, dbConf.Host, dbConf.Port, dbConf.Database)
	fmt.Printf("db_url is :%s", url)
	DB = NewDBClient(dbConf)
}

func NewDBClient(dbConf configs.DBConfig) *ent_work.Client {
	dataSourceName := fmt.Sprintf("host=%s port=%v user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai", dbConf.Host, dbConf.Port, dbConf.Username, dbConf.Password, dbConf.Database)
	logrus.Debugf("dsn: %s\n", dataSourceName)
	db, err := sql.Open("pgx", dataSourceName)
	if err != nil {
		panic(fmt.Sprintf("new db client failed: %v", err))
	}
	db.SetConnMaxLifetime(time.Minute * 10)
	db.SetConnMaxIdleTime(time.Minute * 10)
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)
	if err != nil {
		logrus.Errorf("failed at creating ent client with db %s, err: %v", dataSourceName, err)
		panic(fmt.Sprintf("new db client failed: %v", err))
	}
	drv := entsql.OpenDB(dialect.Postgres, db)
	return ent_work.NewClient(ent_work.Driver(drv))
}
