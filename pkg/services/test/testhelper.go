package test

import (
	"github.com/jinzhu/gorm"
	"log"
	"openpitrix.io/logger"
	"openpitrix.io/notification/pkg/config"
	"openpitrix.io/notification/pkg/util/dbutil"
	"openpitrix.io/notification/pkg/util/etcdutil"
	"os"
)

func GetTestDB4Test() *gorm.DB {
	InitGlobelSetting4Test()
	db := dbutil.GetInstance().GetMysqlDB()
	db.LogMode(true)
	return db
}

func GetEtcdQueue4Test() *etcdutil.Queue {
	cfg := config.GetInstance()
	endpoints := []string{cfg.Etcd.Endpoints}
	prefix := cfg.Etcd.Prefix
	topic := cfg.Etcd.Topic

	nfetcd, err := etcdutil.Connect(endpoints, prefix)
	if err != nil {
		log.Fatal(err)
	}
	q := nfetcd.NewQueue(topic)
	return q
}

func GetTestDBAndEtcd4Test()( *gorm.DB, *etcdutil.Queue) {
	return GetTestDB4Test(),GetEtcdQueue4Test()
}

func InitGlobelSetting4Test() {
	logger.Debugf(nil, "step0.1:初始化配置参数")
	//	config.GetInstance().InitCfg()
	mycfg := config.GetInstance()
	mycfg.LoadConf()

	logger.Debugf(nil, "step0.2:初始化DB connection pool")
	issucc := dbutil.GetInstance().InitDataPool()
	if !issucc {
		logger.Debugf(nil, "init database pool failure...")
		os.Exit(1)
	}

	AppLogMode := mycfg.App.Applogmode
	logger.SetLevelByString(AppLogMode)

}