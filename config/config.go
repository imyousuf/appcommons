package config

import (
	"database/sql"
	"errors"
	"net"
	"os/user"
	"time"

	"github.com/go-ini/ini"

	// MySQL DB Driver
	_ "github.com/go-sql-driver/mysql"
	// SQLite3 DB Driver
	_ "github.com/mattn/go-sqlite3"
)

// DBDialect allows us to define constants for supported DB drivers
type DBDialect string

// LogLevel represents the log level logger should use
type LogLevel uint8

// RelationalDatabaseConfig represents DB configuration related behaviors
type RelationalDatabaseConfig interface {
	GetDBDialect() DBDialect
	GetDBConnectionURL() string
	GetDBConnectionMaxIdleTime() time.Duration
	GetDBConnectionMaxLifetime() time.Duration
	GetMaxIdleDBConnections() uint16
	GetMaxOpenDBConnections() uint16
}

// HTTPConfig represents the HTTP configuration related behaviors
type HTTPConfig interface {
	GetHTTPListeningAddr() string
	GetHTTPReadTimeout() time.Duration
	GetHTTPWriteTimeout() time.Duration
}

// LogConfig represents the interface for log related configuration
type LogConfig interface {
	GetLogLevel() LogLevel
	IsLoggerConfigAvailable() bool
	GetLogFilename() string
	GetMaxLogFileSize() uint
	GetMaxLogBackups() uint
	GetMaxAgeForALogFile() uint
	IsCompressionEnabledOnLogBackups() bool
}

const (
	// SQLite3Dialect represents the DB Dialect for SQLite3
	SQLite3Dialect = DBDialect("sqlite3")
	// MySQLDialect represents the DB Dialect for MySQL
	MySQLDialect = DBDialect("mysql")
	// Debug is the lowest LogLevel, will expose all logs
	Debug LogLevel = 20 + iota
	// Info is the second lowest LogLevel
	Info
	// Error is the second highest LogLevel
	Error
	// Fatal is the highest LogLevel with lowest logs
	Fatal
)

var (
	// EmptyConfigurationForError Represents the configuration instance to be
	// used when there is a configuration error during load
	EmptyConfigurationForError = &Config{}
	// ConfigFilename is the default file name when nothing is provided
	ConfigFilename = "appconfig.cfg"
	// DefaultConfiguration is the configuration that will be in effect if no configuration is loaded from any of the expected locations
	DefaultConfiguration = `[rdbms]
dialect=sqlite3
connection-url=database.sqlite3?_foreign_keys=on
connxn-max-idle-time-seconds=0
connxn-max-lifetime-seconds=0
max-idle-connxns=30
max-open-connxns=100
[http]
listener=:7050
read-timeout=240
write-timeout=240
[log]
filename=
max-file-size-in-mb=200
max-backups=3
max-age-in-days=28
compress-backups=true
log-level=debug
`

	// DefaultLoadFunc defines
	DefaultLoadFunc   = GetLoadFunc(DefaultConfiguration, ConfigFilename, "/etc/appconfig/", "/.appconfig/")
	LoadConfiguration = DefaultLoadFunc
	errDBDialect      = errors.New("DB Dialect not supported")
	// ConfigInjector sets up configuration related bindings
)

var currentUser = user.Current

func getUserHomeDirBasedDefaultConfigFileLocation(pathPrefix, configFileName string) string {
	user, err := currentUser()
	if err != nil {
		return configFileName
	}
	return user.HomeDir + pathPrefix + configFileName
}

// GetLoadFunc provides an API for wrapping multi-level configuration loading for any application trying to load function
func GetLoadFunc(defaultConfig, configFilename, systemPathPrefix, userHomePathPrefix string) func(string) (*ini.File, error) {
	return func(configFilePath string) (*ini.File, error) {
		if len(configFilePath) > 0 {
			return ini.LooseLoad([]byte(defaultConfig), systemPathPrefix+configFilename, getUserHomeDirBasedDefaultConfigFileLocation(userHomePathPrefix, configFilename), configFilename, configFilePath)
		}
		return ini.LooseLoad([]byte(defaultConfig), systemPathPrefix+configFilename, getUserHomeDirBasedDefaultConfigFileLocation(userHomePathPrefix, configFilename), configFilename)
	}
}

//Config represents the application configuration
type Config struct {
	DBDialect               DBDialect
	DBConnectionURL         string
	DBConnectionMaxIdleTime time.Duration
	DBConnectionMaxLifetime time.Duration
	DBMaxIdleConnections    uint16
	DBMaxOpenConnections    uint16
	HTTPListeningAddr       string
	HTTPReadTimeout         time.Duration
	HTTPWriteTimeout        time.Duration
	LogFilename             string
	MaxFileSize             uint
	MaxBackups              uint
	MaxAge                  uint
	CompressBackupsEnabled  bool
	LogLevel                LogLevel
}

// GetLogLevel returns the log level as per the configuration
func (config *Config) GetLogLevel() LogLevel {
	return config.LogLevel
}

// GetDBDialect returns the DB dialect of the configuration
func (config *Config) GetDBDialect() DBDialect {
	return config.DBDialect
}

// GetDBConnectionURL returns the DB Connection URL string
func (config *Config) GetDBConnectionURL() string {
	return config.DBConnectionURL
}

// GetDBConnectionMaxIdleTime returns the DB Connection max idle time
func (config *Config) GetDBConnectionMaxIdleTime() time.Duration {
	return config.DBConnectionMaxIdleTime
}

// GetDBConnectionMaxLifetime returns the DB Connection max lifetime
func (config *Config) GetDBConnectionMaxLifetime() time.Duration {
	return config.DBConnectionMaxLifetime
}

// GetMaxIdleDBConnections returns the maximum number of idle DB connections to retain in pool
func (config *Config) GetMaxIdleDBConnections() uint16 {
	return config.DBMaxIdleConnections
}

// GetMaxOpenDBConnections returns the maximum number of concurrent DB connections to keep open
func (config *Config) GetMaxOpenDBConnections() uint16 {
	return config.DBMaxOpenConnections
}

// GetHTTPListeningAddr retrieves the connection string to listen to
func (config *Config) GetHTTPListeningAddr() string {
	return config.HTTPListeningAddr
}

// GetHTTPReadTimeout retrieves the connection read timeout
func (config *Config) GetHTTPReadTimeout() time.Duration {
	return config.HTTPReadTimeout
}

// GetHTTPWriteTimeout retrieves the connection write timeout
func (config *Config) GetHTTPWriteTimeout() time.Duration {
	return config.HTTPWriteTimeout
}

// IsLoggerConfigAvailable checks is logger configuration is set since its optional
func (config *Config) IsLoggerConfigAvailable() bool {
	return len(config.LogFilename) > 0
}

// GetLogFilename retrieves the file name of the log
func (config *Config) GetLogFilename() string {
	return config.LogFilename
}

// GetMaxLogFileSize retrieves the max log file size before its rotated in MB
func (config *Config) GetMaxLogFileSize() uint {
	return config.MaxFileSize
}

// GetMaxLogBackups retrieves max rotated logs to retain
func (config *Config) GetMaxLogBackups() uint {
	return config.MaxBackups
}

// GetMaxAgeForALogFile retrieves maximum day to retain a rotated log file
func (config *Config) GetMaxAgeForALogFile() uint {
	return config.MaxAge
}

// IsCompressionEnabledOnLogBackups checks if log backups are compressed
func (config *Config) IsCompressionEnabledOnLogBackups() bool {
	return config.CompressBackupsEnabled
}

// func (config *Config) () {}

// GetAutoConfiguration gets configuration from default config and system defined path chain of
// /etc/webhook-broker/webhook-broker.cfg, {USER_HOME}/.webhook-broker/webhook-broker.cfg, webhook-broker.cfg (current dir)
func GetAutoConfiguration() (*Config, *ini.File, error) {
	return GetConfiguration("")
}

// GetConfigurationFromCLIConfig from CLIConfig.
func GetConfigurationFromCLIConfig(cliConfig *CLIConfig) (*Config, *ini.File, error) {
	if len(cliConfig.ConfigPath) > 0 {
		return GetConfiguration(cliConfig.ConfigPath)
	}
	return GetAutoConfiguration()
}

// GetConfiguration gets the current state of application configuration
func GetConfiguration(configFilePath string) (*Config, *ini.File, error) {
	cfg, err := LoadConfiguration(configFilePath)
	if err != nil {
		return EmptyConfigurationForError, nil, err
	}
	conf, err := GetConfigurationFromParseConfig(cfg)
	return conf, cfg, err
}

// GetConfigurationFromParseConfig returns configuration from parsed configuration
func GetConfigurationFromParseConfig(cfg *ini.File) (*Config, error) {
	configuration := &Config{}
	setupStorageConfiguration(cfg, configuration)
	setupHTTPConfiguration(cfg, configuration)
	setupLogConfiguration(cfg, configuration)
	if validationErr := validateConfigurationState(configuration); validationErr != nil {
		return EmptyConfigurationForError, validationErr
	}
	return configuration, nil
}

func validateConfigurationState(configuration *Config) error {
	if len(configuration.HTTPListeningAddr) <= 0 {
		configuration.HTTPListeningAddr = ":8080"
	}
	// Check Listener Address port is open
	ln, netErr := net.Listen("tcp", configuration.HTTPListeningAddr)
	if netErr != nil {
		return netErr
	}
	defer ln.Close()
	// Check DB Connection is valid
	var ping func(*sql.DB) error
	switch configuration.DBDialect {
	case SQLite3Dialect:
		ping = pingSqlite3
	case MySQLDialect:
		ping = pingMysql
	default:
		return errDBDialect
	}
	db, dbConnectionErr := sql.Open(string(configuration.DBDialect), configuration.DBConnectionURL)
	if dbConnectionErr != nil {
		return dbConnectionErr
	}
	defer db.Close()
	db.SetConnMaxLifetime(configuration.DBConnectionMaxLifetime)
	db.SetMaxIdleConns(int(configuration.DBMaxIdleConnections))
	db.SetMaxOpenConns(int(configuration.DBMaxOpenConnections))
	db.SetConnMaxIdleTime(configuration.DBConnectionMaxIdleTime)
	var typicalErr error
	dbErr := ping(db)
	if dbErr != nil {
		typicalErr = dbErr
	}
	return typicalErr
}

var (
	pingSqlite3 = func(db *sql.DB) error {
		rows, queryErr := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		return nil
	}

	pingMysql = func(db *sql.DB) error {
		rows, queryErr := db.Query("SHOW Tables")
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		return nil
	}
)

func setupStorageConfiguration(cfg *ini.File, configuration *Config) {
	dbSection, _ := cfg.GetSection("rdbms")
	dbDialect, _ := dbSection.GetKey("dialect")
	dbConnection, _ := dbSection.GetKey("connection-url")
	dbMaxIdleTimeInSec, _ := dbSection.GetKey("connxn-max-idle-time-seconds")
	dbMaxLifetimeInSec, _ := dbSection.GetKey("connxn-max-lifetime-seconds")
	dbMaxIdleConnections, _ := dbSection.GetKey("max-idle-connxns")
	dbMaxOpenConnections, _ := dbSection.GetKey("max-open-connxns")
	configuration.DBDialect = DBDialect(dbDialect.String())
	configuration.DBConnectionURL = dbConnection.String()
	configuration.DBConnectionMaxIdleTime = time.Duration(dbMaxIdleTimeInSec.MustUint(0)) * time.Second
	configuration.DBConnectionMaxLifetime = time.Duration(dbMaxLifetimeInSec.MustUint(0)) * time.Second
	configuration.DBMaxIdleConnections = uint16(dbMaxIdleConnections.MustUint(10))
	configuration.DBMaxOpenConnections = uint16(dbMaxOpenConnections.MustUint(50))
}

func setupHTTPConfiguration(cfg *ini.File, configuration *Config) {
	httpSection, _ := cfg.GetSection("http")
	httpListener, _ := httpSection.GetKey("listener")
	httpReadTimeout, _ := httpSection.GetKey("read-timeout")
	httpWriteTimeout, _ := httpSection.GetKey("write-timeout")
	configuration.HTTPListeningAddr = httpListener.String()
	configuration.HTTPReadTimeout = time.Duration(httpReadTimeout.MustUint(180)) * time.Second
	configuration.HTTPWriteTimeout = time.Duration(httpWriteTimeout.MustUint(180)) * time.Second
}

func setupLogConfiguration(cfg *ini.File, configuration *Config) {
	logSection, _ := cfg.GetSection("log")
	logFilenameKey, _ := logSection.GetKey("filename")
	maxFileSizeKey, _ := logSection.GetKey("max-file-size-in-mb")
	maxBackupsKey, _ := logSection.GetKey("max-backups")
	maxAgeKey, _ := logSection.GetKey("max-age-in-days")
	compressEnabledKey, _ := logSection.GetKey("compress-backups")
	configuration.LogFilename = logFilenameKey.String()
	configuration.MaxFileSize = maxFileSizeKey.MustUint(50)
	configuration.MaxBackups = maxBackupsKey.MustUint(1)
	configuration.MaxAge = maxAgeKey.MustUint(30)
	configuration.CompressBackupsEnabled = compressEnabledKey.MustBool(false)
	logLevelKey, _ := logSection.GetKey("log-level")
	var logLevel LogLevel
	switch logLevelKey.MustString("debug") {
	case "fatal":
		logLevel = Fatal
	case "error":
		logLevel = Error
	case "info":
		logLevel = Info
	default:
		logLevel = Debug
	}
	configuration.LogLevel = logLevel
}
