package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/imyousuf/appcommons/config"
	"github.com/imyousuf/appcommons/data"
	"github.com/rs/zerolog/log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migrate_mysql "github.com/golang-migrate/migrate/v4/database/mysql"
	migrate_sqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"

	// MySQL DB Driver
	_ "github.com/go-sql-driver/mysql"
	// SQLite3 DB Driver
	_ "github.com/mattn/go-sqlite3"
	// File as a source for migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// MigrationConfig represents the DB migration config
type (
	MigrationConfig struct {
		MigrationEnabled bool
		MigrationSource  string
	}

	// orderByClause represents the string to append for sorting to a DB query
	orderByClause string
	PageSizeEnum  int
)

const (
	baseOrderByClause                   orderByClause = "ORDER BY createdAt desc, id desc"
	pageSizeWithOrder                   orderByClause = baseOrderByClause + " LIMIT 25"
	mediumPageSizeWithOrder             orderByClause = baseOrderByClause + " LIMIT 50"
	largePageSizeWithOrder              orderByClause = baseOrderByClause + " LIMIT 100"
	extraLargePageSizeWithOrder         orderByClause = baseOrderByClause + " LIMIT 500"
	baseOrderByClauseOpposite           orderByClause = "ORDER BY createdAt asc, id asc"
	pageSizeWithOrderOpposite           orderByClause = baseOrderByClauseOpposite + " LIMIT 25"
	mediumPageSizeWithOrderOpposite     orderByClause = baseOrderByClauseOpposite + " LIMIT 50"
	largePageSizeWithOrderOpposite      orderByClause = baseOrderByClauseOpposite + " LIMIT 100"
	extraLargePageSizeWithOrderOpposite orderByClause = baseOrderByClauseOpposite + " LIMIT 500"
	// RegularPageSize represents enum for page size of 25
	RegularPageSize PageSizeEnum = iota
	// MediumPageSize represents enum for page size of 50
	MediumPageSize
	// LargePageSize represents enum for page size of 100
	LargePageSize
	// ExtraLargePageSize represents enum for page size of 500
	ExtraLargePageSize
)

var (
	db                      *sql.DB
	dataAccessorInitializer sync.Once
	// ErrDBConnectionNeverInitialized is returned when same NewDataAccessor is called the first time and it failed to connec to DB; in all subsequent calls the accessor will remain nil
	errDBConnectionNeverInitialized = errors.New("database connection never initialized")
	// ErrNoRowsUpdated is returned when a UPDATE query does not change any row which is unexpected
	ErrNoRowsUpdated = errors.New("no rows updated on UPDATE query")
	// ExpectedMaxRowCount represents the max rows from the preset page sizes
	ExpectedMaxRowCount = map[PageSizeEnum]int{
		ExtraLargePageSize: 500,
		LargePageSize:      100,
		MediumPageSize:     50,
		RegularPageSize:    25,
	}

	// GetConfiguredConnectionPool Retrieves the connection pool to the DB including running of the migration
	GetConfiguredConnectionPool = func(dbConfig config.RelationalDatabaseConfig, migrationConf *MigrationConfig) (*sql.DB, error) {
		var err error = nil
		dataAccessorInitializer.Do(func() {
			// Initialize DB Connection
			db, err = CreateDBConnectionPool(dbConfig)
			if err == nil {
				// Run Migration
				err = runMigration(db, dbConfig, migrationConf)
			}
		})
		if db == nil && err == nil {
			err = errDBConnectionNeverInitialized
		}
		return db, err
	}

	// CreateDBConnectionPool just initializes the connection pool to the DB and does nothing else
	CreateDBConnectionPool = func(dbConfig config.RelationalDatabaseConfig) (*sql.DB, error) {
		db, err := getDB(string(dbConfig.GetDBDialect()), dbConfig.GetDBConnectionURL())
		if err == nil {
			db.SetConnMaxLifetime(dbConfig.GetDBConnectionMaxLifetime())
			db.SetMaxIdleConns(int(dbConfig.GetMaxIdleDBConnections()))
			db.SetMaxOpenConns(int(dbConfig.GetMaxOpenDBConnections()))
			db.SetConnMaxIdleTime(dbConfig.GetDBConnectionMaxIdleTime())
		}
		return db, err
	}

	getDB = func(dialect, connectionURL string) (*sql.DB, error) {
		return sql.Open(string(dialect), connectionURL)
	}
	runMigration = func(db *sql.DB, dbConfig config.RelationalDatabaseConfig, migrationConf *MigrationConfig) error {
		if migrationConf.MigrationEnabled {
			driver, err := getMigrationDriver(db, dbConfig)
			if err != nil {
				return err
			}
			migration, err := getMigration(migrationConf.MigrationSource, string(dbConfig.GetDBDialect()), driver)
			if err != nil {
				return err
			}
			err = migration.Up()
			if err != nil && err != migrate.ErrNoChange {
				return err
			}
		}
		return nil
	}

	getMigration = func(source, dialect string, driver database.Driver) (*migrate.Migrate, error) {
		return migrate.NewWithDatabaseInstance(source, dialect, driver)
	}

	getMigrationDriver = func(db *sql.DB, dbConfig config.RelationalDatabaseConfig) (database.Driver, error) {
		switch dbConfig.GetDBDialect() {
		case config.MySQLDialect:
			return migrate_mysql.WithInstance(db, &migrate_mysql.Config{})
		default:
			return migrate_sqlite3.WithInstance(db, &migrate_sqlite3.Config{})
		}
	}

	// Rollback rolls back the transaction with error logging if any
	Rollback = func(tx *sql.Tx) {
		txErr := tx.Rollback()
		if txErr != nil {
			log.Error().Err(txErr).Msg("tx rollback error")
		}
	}

	// ExecuteOpsInTransaction is the most high level function for wrapping DB Transaction Begin -> Do Queries -> Commit if success or Rollback.
	// It has panic recovery backed in for default rollback
	ExecuteOpsInTransaction = func(db *sql.DB, txOps func(tx *sql.Tx) error) (err error) {
		var tx *sql.Tx
		tx, err = db.Begin()
		defer func() {
			if r := recover(); r != nil {
				log.Error().Msg(fmt.Sprint("recovered from in-tx panic", r))
				Rollback(tx)
			}
		}()
		if err == nil {
			err = txOps(tx)
			if err == nil {
				txErr := tx.Commit()
				if txErr != nil {
					log.Error().Err(txErr).Msg("tx commit error")
					err = txErr
				}
			} else {
				Rollback(tx)
			}
		}
		return err
	}

	// ExecuteQueryInTransaction is a specific helper function designed for executing a write query that may effect multiple rows
	ExecuteQueryInTransaction = func(tx *sql.Tx, prequeryOps func(), query string, arguments func() []interface{}, expectedRowEffected int64) (err error) {
		prequeryOps()
		var result sql.Result
		result, err = tx.Exec(query, arguments()...)
		if err == nil {
			var rowsAffected int64
			if rowsAffected, err = result.RowsAffected(); expectedRowEffected > 0 && rowsAffected != expectedRowEffected && err == nil {
				err = ErrNoRowsUpdated
			}
		}
		return err
	}

	// GetTxWrapperForSingleWriteQuery is a helper for wrapping single query with tx to received at a later time
	GetTxWrapperForSingleWriteQuery = func(prequeryOps func(), query string, arguments func() []interface{}) func(tx *sql.Tx) error {
		return func(tx *sql.Tx) error {
			return ExecuteQueryInTransaction(tx, prequeryOps, query, arguments, int64(1))
		}
	}

	// ExecuteSingleRowWriteInTransaction is a specific helper function designed for executing a write query that should effect exactly one row
	ExecuteSingleRowWriteInTransaction = func(db *sql.DB, prequeryOps func(), query string, arguments func() []interface{}) error {
		return ExecuteMultipleWriteOpsInTransaction(db, GetTxWrapperForSingleWriteQuery(prequeryOps, query, arguments))
	}

	// Allows for multiple write operations to be performed within a single transaction
	ExecuteMultipleWriteOpsInTransaction = func(db *sql.DB, ops ...func(tx *sql.Tx) error) error {
		return ExecuteOpsInTransaction(db, func(tx *sql.Tx) (err error) {
			for _, op := range ops {
				if op == nil {
					log.Warn().Msg("Tx Op is nil! Ignoring it")
					continue
				}
				err = op(tx)
				if err != nil {
					break
				}
			}
			return err
		})
	}

	// GetPaginationQueryFragmentWithConfigurablePageSize generates query substring for being appended to a query. It can either be
	// appended to a existing where clause with append supplied as true or if append is false it will generate a query that can be
	// appended directly after the WHERE clause.
	GetPaginationQueryFragmentWithConfigurablePageSize = func(page *data.Pagination, append bool, pageSize PageSizeEnum) string {
		query := " "
		orderByQueryClause := baseOrderByClause
		switch pageSize {
		case ExtraLargePageSize:
			orderByQueryClause = extraLargePageSizeWithOrder
		case LargePageSize:
			orderByQueryClause = largePageSizeWithOrder
		case MediumPageSize:
			orderByQueryClause = mediumPageSizeWithOrder
		case RegularPageSize:
			orderByQueryClause = pageSizeWithOrder
		default:
			orderByQueryClause = pageSizeWithOrder
		}
		if page.Next != nil {
			if append {
				query = query + "AND "
			} else {
				query = query + "WHERE "
			}
			query = query + "id < '" + page.Next.ID + "' "
			query = query + "AND createdAt <= ? "
		} else if page.Previous != nil {
			if append {
				query = query + "AND "
			} else {
				query = query + "WHERE "
			}
			query = query + "id > '" + page.Previous.ID + "' "
			query = query + "AND createdAt >= ? "
			switch pageSize {
			case ExtraLargePageSize:
				orderByQueryClause = extraLargePageSizeWithOrderOpposite
			case LargePageSize:
				orderByQueryClause = largePageSizeWithOrderOpposite
			case MediumPageSize:
				orderByQueryClause = mediumPageSizeWithOrderOpposite
			case RegularPageSize:
				orderByQueryClause = pageSizeWithOrderOpposite
			default:
				orderByQueryClause = pageSizeWithOrderOpposite
			}
		}
		query = query + string(orderByQueryClause)
		return query
	}

	// GetPaginationQueryFragment is same as GetPaginationQueryFragmentWithConfigurablePageSize but with regular page size as the page size
	GetPaginationQueryFragment = func(page *data.Pagination, append bool) string {
		return GetPaginationQueryFragmentWithConfigurablePageSize(page, append, RegularPageSize)
	}

	// GetPaginationTimestampQueryArgs will generate the arguments pertaining to pagination fragment generated above
	GetPaginationTimestampQueryArgs = func(page *data.Pagination) []interface{} {
		times := make([]interface{}, 0, 2)
		if page.Next != nil {
			times = append(times, page.Next.Timestamp)
		}
		if page.Previous != nil {
			times = append(times, page.Previous.Timestamp)
		}
		return times
	}

	// QuerySingleRow is a helper designed to expect and read a single row from a result set
	QuerySingleRow = func(db *sql.DB, query string, queryArgs func() []interface{}, scanArgs func() []interface{}) error {
		row := db.QueryRow(query, queryArgs()...)
		return row.Scan(scanArgs()...)
	}

	// QuerySingleRow is a helper designed to expect and read multiple rows from a result set
	QueryRows = func(db *sql.DB, query string, queryArgs func() []interface{}, scanArgs func() []interface{}) error {
		rows, err := db.Query(query, queryArgs()...)
		if err != nil {
			return err
		}
		defer func() { rows.Close() }()
		for rows.Next() {
			err = rows.Scan(scanArgs()...)
			if err != nil {
				return err
			}
		}
		return err
	}

	// AppendWithPaginationArgs appends query positional arguments for pagination to the existing list of positional arguments
	AppendWithPaginationArgs = func(page *data.Pagination, args ...interface{}) []interface{} {
		return append(args, GetPaginationTimestampQueryArgs(page)...)
	}

	// NilArgs is placeholder for cases where no args are needed for a query
	NilArgs = func() []interface{} { return nil }

	// EmptyOps is a blank function to be used as placeholder
	EmptyOps = func() {}

	// Args2SliceFnWrapper is a helper function to convert var-args to a slice
	Args2SliceFnWrapper = func(args ...interface{}) func() []interface{} {
		return func() []interface{} { return args }
	}
)
