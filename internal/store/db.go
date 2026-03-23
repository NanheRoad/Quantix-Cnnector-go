package store

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"quantix-connector-go/internal/config"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func OpenDB(cfg config.Settings) (*gorm.DB, error) {
	var (
		db  *gorm.DB
		err error
	)
	gcfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

	if strings.ToLower(cfg.DBType) == "mysql" {
		mysqlCfg := mysqlDriver.NewConfig()
		mysqlCfg.User = cfg.DBUser
		mysqlCfg.Passwd = cfg.DBPassword
		mysqlCfg.Net = "tcp"
		mysqlCfg.Addr = net.JoinHostPort(cfg.DBHost, strconv.Itoa(cfg.DBPort))
		mysqlCfg.DBName = cfg.DBName
		mysqlCfg.ParseTime = true
		mysqlCfg.Loc = time.Local
		mysqlCfg.Params = map[string]string{"charset": "utf8mb4"}
		dsn := mysqlCfg.FormatDSN()
		db, err = gorm.Open(mysql.Open(dsn), gcfg)
	} else {
		db, err = gorm.Open(sqlite.Open(cfg.DBName), gcfg)
	}
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	if strings.ToLower(cfg.DBType) == "mysql" {
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(10)
	} else {
		// SQLite favors a single writer connection to reduce lock contention.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}

	if err := db.AutoMigrate(&ProtocolTemplate{}, &Device{}); err != nil {
		return nil, err
	}
	if err := ensureSeed(db); err != nil {
		return nil, err
	}
	if err := ensureDeviceCodeAndCategory(db); err != nil {
		return nil, err
	}
	return db, nil
}

func ensureDeviceCodeAndCategory(db *gorm.DB) error {
	var devices []Device
	if err := db.Order("id asc").Find(&devices).Error; err != nil {
		return err
	}
	used := map[string]struct{}{}
	for i := range devices {
		d := &devices[i]
		code, err := NormalizeDeviceCode(d.DeviceCode)
		if err != nil {
			code = BuildDefaultDeviceCode(d.ID)
		}
		category, err := NormalizeDeviceCategory(d.DeviceCategory)
		if err != nil {
			category = "weight"
		}
		candidate := code
		suffix := 1
		for {
			if _, ok := used[candidate]; !ok {
				break
			}
			sfx := fmt.Sprintf("-%d", suffix)
			base := code
			if len(base)+len(sfx) > 64 {
				base = base[:64-len(sfx)]
			}
			candidate = base + sfx
			suffix++
		}
		used[candidate] = struct{}{}
		if d.DeviceCode != candidate || d.DeviceCategory != category {
			if err := db.Model(d).Updates(map[string]any{"device_code": candidate, "device_category": category}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
