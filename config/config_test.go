package config

import (
	"database/sql"
	"errors"
	"net"
	"os/user"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-ini/ini"
	"github.com/stretchr/testify/assert"
)

const (
	wrongValueConfig = `[rdbms]
	dialect=sqlite3
	connection-url=database.sqlite3?_foreign_keys=on
	connxn-max-idle-time-seconds=-10
	connxn-max-lifetime-seconds=ascx0x
	max-idle-connxns=as30
	max-open-connxns=-100
	[http]
	listener=:7050
	read-timeout=asd240
	write-timeout=zf240
	[log]
	filename=/var/log/webhook-broker.log
	max-file-size-in-mb=as200
	max-backups=asd3
	max-age-in-days=dasd28
	compress-backups=asdtrue
	log-level=random
	`
	errorConfig = `[rdbms]
	asda sdads
	connection-url=webhook_broker:zxc909zxc@tcp(mysql:3306)/webhook-broker?charset=utf8&parseTime=True
	`
)

var (
	loadTestConfiguration = func(testConfiguration string) *ini.File {
		cfg, _ := ini.LooseLoad([]byte(DefaultConfiguration), []byte(testConfiguration))
		return cfg
	}
)

func toSecond(second uint) time.Duration {
	return time.Duration(second) * time.Second
}

func TestGetAutoConfiguration_Default(t *testing.T) {
	config, cfgErr := GetAutoConfiguration()
	if cfgErr != nil {
		t.Error("Auto Configuration failed", cfgErr)
		t.Fail()
	}
	assert.Equal(t, SQLite3Dialect, config.GetDBDialect())
	assert.Equal(t, "database.sqlite3?_foreign_keys=on", config.GetDBConnectionURL())
	assert.Equal(t, time.Duration(0), config.GetDBConnectionMaxIdleTime())
	assert.Equal(t, time.Duration(0), config.GetDBConnectionMaxLifetime())
	assert.Equal(t, uint16(30), config.GetMaxIdleDBConnections())
	assert.Equal(t, uint16(100), config.GetMaxOpenDBConnections())
	assert.Equal(t, ":7050", config.GetHTTPListeningAddr())
	assert.Equal(t, toSecond(uint(240)), config.GetHTTPReadTimeout())
	assert.Equal(t, toSecond(uint(240)), config.GetHTTPWriteTimeout())
	assert.Equal(t, "", config.GetLogFilename())
	assert.Equal(t, Debug, config.GetLogLevel())
	assert.Equal(t, uint(200), config.GetMaxLogFileSize())
	assert.Equal(t, uint(28), config.GetMaxAgeForALogFile())
	assert.Equal(t, uint(3), config.GetMaxLogBackups())
	assert.Equal(t, true, config.IsCompressionEnabledOnLogBackups())
	assert.Equal(t, false, config.IsLoggerConfigAvailable())
}

func TestGetAutoConfiguration_WrongValues(t *testing.T) {
	LoadConfiguration = func(location string) (*ini.File, error) {
		return ini.InsensitiveLoad([]byte(wrongValueConfig))
	}
	config, cfgErr := GetAutoConfiguration()
	if cfgErr != nil {
		t.Error("Auto Configuration failed", cfgErr)
		t.Fail()
	}
	assert.Equal(t, time.Duration(0), config.GetDBConnectionMaxIdleTime())
	assert.Equal(t, time.Duration(0), config.GetDBConnectionMaxLifetime())
	assert.Equal(t, uint16(10), config.GetMaxIdleDBConnections())
	assert.Equal(t, uint16(50), config.GetMaxOpenDBConnections())
	assert.Equal(t, ":7050", config.GetHTTPListeningAddr())
	assert.Equal(t, toSecond(uint(180)), config.GetHTTPReadTimeout())
	assert.Equal(t, toSecond(uint(180)), config.GetHTTPWriteTimeout())
	assert.Equal(t, "/var/log/webhook-broker.log", config.GetLogFilename())
	assert.Equal(t, Debug, config.GetLogLevel())
	assert.Equal(t, uint(50), config.GetMaxLogFileSize())
	assert.Equal(t, uint(30), config.GetMaxAgeForALogFile())
	assert.Equal(t, uint(1), config.GetMaxLogBackups())
	assert.Equal(t, false, config.IsCompressionEnabledOnLogBackups())
	assert.Equal(t, true, config.IsLoggerConfigAvailable())
	defer func() {
		LoadConfiguration = DefaultLoadFunc
	}()
}

func TestGetAutoConfiguration_LoadConfigurationError(t *testing.T) {
	LoadConfiguration = func(location string) (*ini.File, error) {
		return ini.InsensitiveLoad([]byte(errorConfig))
	}
	config, cfgErr := GetAutoConfiguration()
	if cfgErr == nil {
		t.Error("Auto Configuration should have failed")
	}
	assert.Equal(t, EmptyConfigurationForError, config)
	defer func() {
		LoadConfiguration = DefaultLoadFunc
	}()
}

func TestGetAutoConfiguration_CurrentUserError(t *testing.T) {
	oldCurrentUser := currentUser
	currentUser = func() (*user.User, error) {
		return nil, errors.New("Unit test error")
	}
	_, cfgErr := GetAutoConfiguration()
	if cfgErr != nil {
		t.Error("Auto Configuration failed", cfgErr)
	}
	defer func() {
		currentUser = oldCurrentUser
	}()
}

func TestGetConfiguration(t *testing.T) {
	config, cfgErr := GetConfiguration("./test-appconfig.cfg")
	if cfgErr != nil {
		t.Error("Auto Configuration failed", cfgErr)
		t.Fail()
	}
	assert.Equal(t, SQLite3Dialect, config.GetDBDialect())
	assert.Equal(t, "database.sqlite3", config.GetDBConnectionURL())
	assert.Equal(t, toSecond(10), config.GetDBConnectionMaxIdleTime())
	assert.Equal(t, toSecond(10), config.GetDBConnectionMaxLifetime())
	assert.Equal(t, uint16(300), config.GetMaxIdleDBConnections())
	assert.Equal(t, uint16(1000), config.GetMaxOpenDBConnections())
	assert.Equal(t, ":7080", config.GetHTTPListeningAddr())
	assert.Equal(t, toSecond(uint(2401)), config.GetHTTPReadTimeout())
	assert.Equal(t, toSecond(uint(2401)), config.GetHTTPWriteTimeout())
	assert.Equal(t, "/var/log/webhook-broker.log", config.GetLogFilename())
	assert.Equal(t, Error, config.GetLogLevel())
	assert.Equal(t, uint(20), config.GetMaxLogFileSize())
	assert.Equal(t, uint(280), config.GetMaxAgeForALogFile())
	assert.Equal(t, uint(30), config.GetMaxLogBackups())
	assert.Equal(t, false, config.IsCompressionEnabledOnLogBackups())
	assert.Equal(t, true, config.IsLoggerConfigAvailable())

	testConfig := `[log]
	log-level=info
	`
	config, err := GetConfigurationFromParseConfig(loadTestConfiguration(testConfig))
	assert.Equal(t, Info, config.GetLogLevel())
	assert.Nil(t, err)
	testConfig = `[log]
	log-level=fatal
	`
	config, err = GetConfigurationFromParseConfig(loadTestConfiguration(testConfig))
	assert.Equal(t, Fatal, config.GetLogLevel())
	assert.Nil(t, err)
}

func TestGetConfigurationFromParseConfig_ValueError(t *testing.T) {
	// Do not make it parallel
	t.Run("ConfigErrorDueToSQLlite3", func(t *testing.T) {
		oldPingSqlite3 := pingSqlite3
		dbErr := errors.New("db error")
		pingSqlite3 = func(db *sql.DB) error {
			return dbErr
		}
		config, err := GetConfigurationFromParseConfig(loadTestConfiguration("[testConfig]"))
		assert.Equal(t, EmptyConfigurationForError, config)
		assert.Equal(t, dbErr, err)
		pingSqlite3 = oldPingSqlite3
	})
	// Do not make it parallel
	t.Run("ConfigErrorDueToMySQL", func(t *testing.T) {
		oldPingMysql := pingMysql
		dbErr := errors.New("db error")
		pingMysql = func(db *sql.DB) error {
			return dbErr
		}
		testConfig := `[rdbms]
		dialect=mysql
		connection-url=webhook_broker:zxc909zxc@tcp(mysql:3306)/webhook-broker?charset=utf8&parseTime=true
		`
		config, err := GetConfigurationFromParseConfig(loadTestConfiguration(testConfig))
		assert.Equal(t, EmptyConfigurationForError, config)
		assert.Equal(t, dbErr, err)
		pingMysql = oldPingMysql
	})
	t.Run("DBDialectNotSupported", func(t *testing.T) {
		t.Parallel()
		testConfig := `[rdbms]
		dialect=mockdb
		[http]
		listener=:48080
		`
		config, err := GetConfigurationFromParseConfig(loadTestConfiguration(testConfig))
		assert.Equal(t, EmptyConfigurationForError, config)
		assert.NotNil(t, err)
		assert.Equal(t, errDBDialect, err)
	})
	t.Run("DBConnectionError", func(t *testing.T) {
		t.Parallel()
		testConfig := `[rdbms]
		dialect=mysql
		connection-url=expect dsn error
		[http]
		listener=:48090
		`
		config, err := GetConfigurationFromParseConfig(loadTestConfiguration(testConfig))
		assert.Equal(t, EmptyConfigurationForError, config)
		assert.NotNil(t, err)
	})
	t.Run("HTTPListenerNotAvailable", func(t *testing.T) {
		t.Parallel()
		testConfig := `
		[http]
		listener=:47070
		`
		ln, netErr := net.Listen("tcp", ":47070")
		if netErr == nil {
			defer ln.Close()
			config, err := GetConfigurationFromParseConfig(loadTestConfiguration(testConfig))
			assert.Equal(t, EmptyConfigurationForError, config)
			assert.NotNil(t, err)
		}
	})
	t.Run("DBPingErrorSQLite3", func(t *testing.T) {
		t.Parallel()
		db, mock, _ := sqlmock.New()
		mockedErr := errors.New("mock db error")
		mock.ExpectQuery("SELECT name FROM sqlite_master WHERE type='table'").WillReturnError(mockedErr)
		err := pingSqlite3(db)
		assert.Equal(t, mockedErr, err)
	})
	t.Run("DBPingErrorMySQL", func(t *testing.T) {
		t.Parallel()
		db, mock, _ := sqlmock.New()
		mockedErr := errors.New("mock db error")
		mock.ExpectQuery("SHOW Tables").WillReturnError(mockedErr)
		err := pingMysql(db)
		assert.Equal(t, mockedErr, err)
	})
	t.Run("DBPingMySQL", func(t *testing.T) {
		t.Parallel()
		db, mock, _ := sqlmock.New()
		rows := sqlmock.NewRows([]string{"ID", "Table Name"})
		mock.ExpectQuery("SHOW Tables").WillReturnRows(rows)
		err := pingMysql(db)
		assert.Nil(t, err)
	})
}

func TestGetConfigurationFromCLIConfig(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		_, err := GetConfigurationFromCLIConfig(&CLIConfig{})
		assert.Nil(t, err)
	})
	t.Run("WithPath", func(t *testing.T) {
		_, err := GetConfigurationFromCLIConfig(&CLIConfig{ConfigPath: "./test-webhook-broker.cfg"})
		assert.Nil(t, err)
	})
}

func TestMigrationEnabled(t *testing.T) {
	t.Run("MigrationEnabled", func(t *testing.T) {
		t.Parallel()
		cliConfig := &CLIConfig{}
		assert.False(t, cliConfig.IsMigrationEnabled())
	})
	t.Run("MigrationDisabled", func(t *testing.T) {
		t.Parallel()
		cliConfig := &CLIConfig{MigrationSource: "file:///test/"}
		assert.True(t, cliConfig.IsMigrationEnabled())
	})
}

func TestConfigInterfaces(t *testing.T) {
	var _ RelationalDatabaseConfig = (*Config)(nil)
	var _ HTTPConfig = (*Config)(nil)
	var _ LogConfig = (*Config)(nil)
}
