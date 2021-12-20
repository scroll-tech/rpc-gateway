package mysql

import (
	"fmt"
	stdLog "log"
	"os"
	"strings"
	"time"

	"github.com/conflux-chain/conflux-infura/store"
	"gorm.io/driver/mysql"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// Config represents the mysql configurations to open a database instance.
type Config struct {
	Host     string
	Username string
	Password string
	Database string

	ConnMaxLifetime time.Duration
	MaxOpenConns    int
	MaxIdleConns    int
}

// NewConfigFromViper creates an instance of Config from Viper.
func NewConfigFromViper() (Config, bool) {
	if !viper.GetBool("store.mysql.enabled") {
		return Config{}, false
	}

	return Config{
		Host:     viper.GetString("store.mysql.host"),
		Username: viper.GetString("store.mysql.username"),
		Password: viper.GetString("store.mysql.password"),
		Database: viper.GetString("store.mysql.database"),

		ConnMaxLifetime: viper.GetDuration("store.mysql.connMaxLifeTime"),
		MaxOpenConns:    viper.GetInt("store.mysql.maxOpenConns"),
		MaxIdleConns:    viper.GetInt("store.mysql.maxIdleConns"),
	}, true
}

// MustOpenOrCreate creates an instance of store or exits on any erorr.
func (config *Config) MustOpenOrCreate(option StoreOption) store.Store {
	newCreated := config.mustCreateDatabaseIfAbsent()

	db := config.mustNewDB(config.Database)

	if newCreated {
		if err := db.Migrator().CreateTable(&transaction{}, &block{}, &log{}, &epochStats{}, &conf{}); err != nil {
			logrus.WithError(err).Fatal("Failed to create tables")
		}

		if err := initLogsPartitions(db); err != nil {
			logrus.WithError(err).Fatal("Failed to init logs table partitions")
		}
	}

	if sqlDb, err := db.DB(); err != nil {
		logrus.WithError(err).Fatal("Failed to init mysql db")
	} else {
		sqlDb.SetConnMaxLifetime(config.ConnMaxLifetime)
		sqlDb.SetMaxOpenConns(config.MaxOpenConns)
		sqlDb.SetMaxIdleConns(config.MaxIdleConns)
	}

	logrus.Info("MySQL database initialized")

	return mustNewStore(db, option)
}

func (config *Config) mustNewDB(database string) *gorm.DB {
	logrusLogLevel := logrus.GetLevel()
	gLogLevel := gormLogger.Warn

	switch { // map log level of logrus to that of gorm
	case logrusLogLevel <= logrus.ErrorLevel:
		gLogLevel = gormLogger.Error
	case logrusLogLevel >= logrus.DebugLevel:
		gLogLevel = gormLogger.Info // gorm info log level is kind of too verbose
	}

	// create gorm logger by customizing the default logger
	gLogger := gormLogger.New(
		stdLog.New(os.Stdout, "\r\n", stdLog.LstdFlags), // io writer
		gormLogger.Config{
			SlowThreshold:             time.Millisecond * 200, // slow SQL threshold (200ms)
			LogLevel:                  gLogLevel,              // log level
			IgnoreRecordNotFoundError: true,                   // never logging on ErrRecordNotFound error, otherwise logs may grow exploded
			Colorful:                  true,                   // use colorful print
		},
	)

	// refer to https://github.com/go-sql-driver/mysql#dsn-data-source-name
	dsn := fmt.Sprintf("%v:%v@tcp(%v)/%v?parseTime=true", config.Username, config.Password, config.Host, database)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gLogger,
	})

	if err != nil {
		logrus.WithError(err).Fatal("Failed to open mysql")
	}

	return db
}

func (config *Config) mustCreateDatabaseIfAbsent() bool {
	db := config.mustNewDB("")
	if mysqlDb, err := db.DB(); err != nil {
		return false
	} else {
		defer mysqlDb.Close()
	}

	rows, err := db.Raw(fmt.Sprintf("SHOW DATABASES LIKE '%v'", config.Database)).Rows()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to query databases")
	}
	defer rows.Close()

	if rows.Next() {
		return false
	}

	if err = db.Exec("CREATE DATABASE IF NOT EXISTS " + config.Database).Error; err != nil {
		logrus.WithError(err).Fatal("Failed to create database")
	}

	logrus.Info("Create database for the first time")

	return true
}

func initLogsPartitions(db *gorm.DB) error {
	sqlLines := make([]string, 0, 120)
	sqlLines = append(sqlLines, "ALTER TABLE logs PARTITION BY RANGE (id)(")

	for i := uint64(0); i < uint64(LogsTablePartitionsNum); i++ {
		lineStr := fmt.Sprintf("PARTITION logs%v VALUES LESS THAN (%v),", i, (i+1)*LogsTablePartitionRangeSize)
		sqlLines = append(sqlLines, lineStr)
	}

	sqlLines = append(sqlLines, "PARTITION logsow VALUES LESS THAN MAXVALUE);")

	logsPartitionSql := strings.Join(sqlLines, "\n")
	logrus.WithField("logsPartitionSql", logsPartitionSql).Debug("Init logs db table partitions")

	return db.Exec(logsPartitionSql).Error
}
