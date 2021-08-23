package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rs/zerolog/log"

	"github.com/imyousuf/appcommons/config"
	"github.com/imyousuf/appcommons/data"
	"github.com/stretchr/testify/assert"
)

const (
	insertQuery              = "INSERT INTO test (id, name, note, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)"
	singleRowRead            = "SELECT id from test ORDER BY id ASC LIMIT 1"
	singleReadCreateRowCount = 25
)

var (
	migrationLocation, _ = filepath.Abs("./test-migration/")
	defaultMigrationConf = &MigrationConfig{MigrationEnabled: true, MigrationSource: "file://" + migrationLocation}
	configuration        *config.Config
	testDB               *sql.DB
)

func TestMain(m *testing.M) {
	// Setup DB and migration
	os.Remove("./database.sqlite3")
	configuration, _, _ = config.GetAutoConfiguration()
	var dbErr error
	testDB, dbErr = GetConfiguredConnectionPool(configuration, defaultMigrationConf)
	if dbErr == nil {
		// Global Test Setups if
		os.Exit(m.Run())
	} else {
		log.Fatal().Err(dbErr)
		os.Exit(101)
	}
	testDB.Close()
}

func TestConnectionInitialized(t *testing.T) {
	var singleRow string
	err := QuerySingleRow(testDB, singleRowRead, NilArgs, Args2SliceFnWrapper(&singleRow))
	assert.NotNil(t, err)
	assert.Equal(t, "", singleRow)
	err = QueryRows(testDB, singleRowRead, NilArgs, Args2SliceFnWrapper(&singleRow))
	assert.Nil(t, err)
	assert.Equal(t, "", singleRow)
}

func TestSequentialInsertionWithSingleRowRead(t *testing.T) {
	count := singleReadCreateRowCount
	var rowData []data.BasePaginateable = make([]data.BasePaginateable, count)
	t.Run("InsertionInLoop", func(t *testing.T) {
		index := 0
		for index < count {
			p := data.BasePaginateable{}
			p.QuickFix()
			rowData[index] = p
			writeErr := ExecuteSingleRowWriteInTransaction(testDB, EmptyOps, insertQuery,
				Args2SliceFnWrapper(p.ID, strconv.Itoa(index), strconv.Itoa(index), p.CreatedAt, p.UpdatedAt))
			assert.Nil(t, writeErr)
			index++
		}
	})
	t.Run("SingleRowRead", func(t *testing.T) {
		var singleRow string
		err := QuerySingleRow(testDB, singleRowRead, NilArgs, Args2SliceFnWrapper(&singleRow))
		assert.Nil(t, err)
		// The primary reason for the following match is XID generating IDs in sequence
		assert.Equal(t, rowData[0].ID.String(), singleRow)
	})
}

func TestSingleTxMultipleInsertionWithPaginationRead(t *testing.T) {
	count := 200
	t.Run("InsertInSingleTx", func(t *testing.T) {
		var txOps []func(*sql.Tx) error = make([]func(*sql.Tx) error, 0, count)
		var index = 0
		for index < count {
			p := &data.BasePaginateable{}
			p.QuickFix()
			txOps = append(txOps, GetTxWrapperForSingleWriteQuery(EmptyOps, insertQuery, Args2SliceFnWrapper(p.ID, strconv.Itoa(index), strconv.Itoa(index), p.CreatedAt, p.UpdatedAt)))
			index++
		}
		err := ExecuteMultipleWriteOpsInTransaction(testDB, txOps...)
		assert.Nil(t, err)
	})
	t.Run("ReadMultipleRowsNoAppend", func(t *testing.T) {
		for _, pageSize := range []PageSizeEnum{RegularPageSize, MediumPageSize, LargePageSize, ExtraLargePageSize} {
			var testData []data.BasePaginateable = make([]data.BasePaginateable, 0, 500)
			page := &data.Pagination{}
			hasMorePage := true
			var absLastData *data.BasePaginateable
			for hasMorePage {
				thisPageCount := 0
				var lastData *data.BasePaginateable
				baseQuery := "SELECT id, createdAt, updatedAt FROM `test`" + GetPaginationQueryFragmentWithConfigurablePageSize(page, false, pageSize)
				log.Debug().Msg(baseQuery)
				scanArgs := func() []interface{} {
					thisPageCount++
					testDatum := data.BasePaginateable{}
					lastData = &testDatum
					absLastData = lastData
					testData = append(testData, testDatum)
					return []interface{}{&testDatum.ID, &testDatum.CreatedAt, &testDatum.UpdatedAt}
				}
				err := QueryRows(testDB, baseQuery, Args2SliceFnWrapper(GetPaginationTimestampQueryArgs(page)...), scanArgs)
				if thisPageCount < ExpectedMaxRowCount[pageSize] {
					hasMorePage = false
				} else {
					page = data.NewPagination(lastData, nil)
				}
				assert.Nil(t, err)
			}
			assert.GreaterOrEqual(t, len(testData), count)
			assert.LessOrEqual(t, len(testData), count+singleReadCreateRowCount)
			var oppositeOrder []data.BasePaginateable = make([]data.BasePaginateable, 0, 500)
			hasMorePage = true
			page = data.NewPagination(nil, absLastData)
			for hasMorePage {
				thisPageCount := 0
				var firstData *data.BasePaginateable
				baseQuery := "SELECT id, createdAt, updatedAt FROM `test`" + GetPaginationQueryFragmentWithConfigurablePageSize(page, false, pageSize)
				log.Debug().Msg(baseQuery)
				scanArgs := func() []interface{} {
					thisPageCount++
					testDatum := data.BasePaginateable{}
					firstData = &testDatum
					oppositeOrder = append(oppositeOrder, testDatum)
					return []interface{}{&testDatum.ID, &testDatum.CreatedAt, &testDatum.UpdatedAt}
				}
				err := QueryRows(testDB, baseQuery, Args2SliceFnWrapper(GetPaginationTimestampQueryArgs(page)...), scanArgs)
				if thisPageCount < ExpectedMaxRowCount[pageSize] {
					hasMorePage = false
				} else {
					page = data.NewPagination(nil, firstData)
				}
				assert.Nil(t, err)
			}
			assert.GreaterOrEqual(t, len(oppositeOrder), count)
			assert.LessOrEqual(t, len(oppositeOrder), count+singleReadCreateRowCount)
		}
	})
	t.Run("ReadMultipleRowsWithAppend", func(t *testing.T) {
		for _, pageSize := range []PageSizeEnum{RegularPageSize, MediumPageSize, LargePageSize, ExtraLargePageSize} {
			var testData []data.BasePaginateable = make([]data.BasePaginateable, 0, 500)
			page := &data.Pagination{}
			hasMorePage := true
			var absLastData *data.BasePaginateable
			for hasMorePage {
				thisPageCount := 0
				var lastData *data.BasePaginateable
				baseQuery := "SELECT id, createdAt, updatedAt FROM `test` WHERE 1 = 1" + GetPaginationQueryFragmentWithConfigurablePageSize(page, true, pageSize)
				log.Debug().Msg(baseQuery)
				scanArgs := func() []interface{} {
					thisPageCount++
					testDatum := data.BasePaginateable{}
					lastData = &testDatum
					absLastData = lastData
					testData = append(testData, testDatum)
					return []interface{}{&testDatum.ID, &testDatum.CreatedAt, &testDatum.UpdatedAt}
				}
				err := QueryRows(testDB, baseQuery, Args2SliceFnWrapper(GetPaginationTimestampQueryArgs(page)...), scanArgs)
				if thisPageCount < ExpectedMaxRowCount[pageSize] {
					hasMorePage = false
				} else {
					page = data.NewPagination(lastData, nil)
				}
				assert.Nil(t, err)
			}
			assert.GreaterOrEqual(t, len(testData), count)
			assert.LessOrEqual(t, len(testData), count+singleReadCreateRowCount)
			var oppositeOrder []data.BasePaginateable = make([]data.BasePaginateable, 0, 500)
			hasMorePage = true
			page = data.NewPagination(nil, absLastData)
			for hasMorePage {
				thisPageCount := 0
				var firstData *data.BasePaginateable
				baseQuery := "SELECT id, createdAt, updatedAt FROM `test` WHERE 1 = 1" + GetPaginationQueryFragmentWithConfigurablePageSize(page, true, pageSize)
				log.Debug().Msg(baseQuery)
				scanArgs := func() []interface{} {
					thisPageCount++
					testDatum := data.BasePaginateable{}
					firstData = &testDatum
					oppositeOrder = append(oppositeOrder, testDatum)
					return []interface{}{&testDatum.ID, &testDatum.CreatedAt, &testDatum.UpdatedAt}
				}
				err := QueryRows(testDB, baseQuery, Args2SliceFnWrapper(GetPaginationTimestampQueryArgs(page)...), scanArgs)
				if thisPageCount < ExpectedMaxRowCount[pageSize] {
					hasMorePage = false
				} else {
					page = data.NewPagination(nil, firstData)
				}
				assert.Nil(t, err)
			}
			assert.GreaterOrEqual(t, len(oppositeOrder), count)
			assert.LessOrEqual(t, len(oppositeOrder), count+singleReadCreateRowCount)
		}
	})
}
