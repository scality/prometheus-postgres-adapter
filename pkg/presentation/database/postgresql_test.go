//nolint:err113,ireturn // I think err113 is bugged in v1.64.8
package database_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"prometheus-postgres-adapter/pkg/presentation/database"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockPool is our mock implementation of pgxpool.Pool.
type MockPool struct {
	mock.Mock
}

func (m *MockPool) Close() {
	m.Called()
}

func (m *MockPool) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	callArgs := m.Called(ctx, sql, args)
	if rows := callArgs.Get(0); rows != nil {
		return rows.(pgx.Rows), callArgs.Error(1)
	}

	return nil, callArgs.Error(1)
}

func (m *MockPool) Begin(ctx context.Context) (pgx.Tx, error) {
	args := m.Called(ctx)
	if tx := args.Get(0); tx != nil {
		return tx.(pgx.Tx), args.Error(1)
	}

	return nil, args.Error(1)
}

func (m *MockPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	callArgs := m.Called(ctx, sql, args)
	if tag := callArgs.Get(0); tag != nil {
		return tag.(pgconn.CommandTag), callArgs.Error(1)
	}

	return pgconn.CommandTag{}, callArgs.Error(1)
}

// MockRows for testing row results.
type MockRows struct {
	mock.Mock
}

func (m *MockRows) Next() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockRows) Scan(dest ...any) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockRows) Close() {
	m.Called()
}

func (m *MockRows) Err() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRows) CommandTag() pgconn.CommandTag {
	args := m.Called()
	return args.Get(0).(pgconn.CommandTag)
}

func (m *MockRows) FieldDescriptions() []pgconn.FieldDescription {
	args := m.Called()
	return args.Get(0).([]pgconn.FieldDescription)
}

// MockTx for testing transactions.
type MockTx struct {
	mock.Mock
}

func (m *MockTx) Begin(ctx context.Context) (pgx.Tx, error) {
	args := m.Called(ctx)
	return args.Get(0).(pgx.Tx), args.Error(1)
}

func (m *MockTx) Commit(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockTx) Rollback(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	args := m.Called(ctx, tableName, columnNames, rowSrc)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	args := m.Called(ctx, b)
	return args.Get(0).(pgx.BatchResults)
}

func (m *MockTx) LargeObjects() pgx.LargeObjects {
	args := m.Called()
	return args.Get(0).(pgx.LargeObjects)
}

func (m *MockTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	args := m.Called(ctx, name, sql)
	if stmt := args.Get(0); stmt != nil {
		return stmt.(*pgconn.StatementDescription), args.Error(1)
	}

	return nil, args.Error(1)
}

func (m *MockTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *MockTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *MockTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

// Add this to your MockTx struct.
func (m *MockTx) Conn() *pgx.Conn {
	args := m.Called()
	if conn := args.Get(0); conn != nil {
		return conn.(*pgx.Conn)
	}

	return nil
}

// createPostgreSQLWithMock creates a PostgreSQL with our mock pool.
func createPostgreSQLWithMock(mockPool *MockPool) *database.PostgreSQL {
	return database.NewPostgreSQL(slog.New(slog.NewTextHandler(io.Discard, nil)), mockPool)
}

func TestPostgreSQL_Close(t *testing.T) {
	mockPool := new(MockPool)
	mockPool.On("Close").Return()

	pg := createPostgreSQLWithMock(mockPool)
	err := pg.Close()

	assert.NoError(t, err)
	mockPool.AssertExpectations(t)
}

func TestPostgreSQL_CheckHealth(t *testing.T) {
	t.Run("successful health check", func(t *testing.T) {
		mockPool := new(MockPool)
		mockPool.On("Ping", mock.Anything).Return(nil)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.CheckHealth(context.Background())

		assert.NoError(t, err)
		mockPool.AssertExpectations(t)
	})

	t.Run("failed health check", func(t *testing.T) {
		mockPool := new(MockPool)
		expectedErr := errors.New("database connection failed")
		mockPool.On("Ping", mock.Anything).Return(expectedErr)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.CheckHealth(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to ping database")
		mockPool.AssertExpectations(t)
	})
}

func TestPostgreSQL_Exec(t *testing.T) {
	t.Run("successful exec", func(t *testing.T) {
		mockPool := new(MockPool)
		cmdTag := pgconn.NewCommandTag("INSERT 0 1")
		mockPool.On("Exec", mock.Anything, "INSERT INTO test VALUES ($1)", mock.Anything).Return(cmdTag, nil)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.Exec(context.Background(), "INSERT INTO test VALUES ($1)", 123)

		assert.NoError(t, err)
		mockPool.AssertExpectations(t)
	})

	t.Run("exec error", func(t *testing.T) {
		mockPool := new(MockPool)
		expectedErr := errors.New("exec error")
		mockPool.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(pgconn.CommandTag{}, expectedErr)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.Exec(context.Background(), "INSERT INTO test VALUES ($1)", 123)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute query")
		mockPool.AssertExpectations(t)
	})
}

func TestPostgreSQL_CopyRows(t *testing.T) {
	t.Run("successful copy", func(t *testing.T) {
		mockPool := new(MockPool)
		mockTx := new(MockTx)

		tableName := "test_table"
		columnNames := []string{"col1", "col2"}
		rows := [][]any{
			{"value1", 1},
			{"value2", 2},
		}

		mockPool.On("Begin", mock.Anything).Return(mockTx, nil)
		mockTx.On("CopyFrom", mock.Anything, pgx.Identifier{tableName}, columnNames, mock.Anything).Return(int64(2), nil)
		mockTx.On("Commit", mock.Anything).Return(nil)
		mockTx.On("Rollback", mock.Anything).Return(nil)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.CopyRows(context.Background(), tableName, columnNames, rows)

		assert.NoError(t, err)
		mockPool.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})
	t.Run("begin transaction error", func(t *testing.T) {
		mockPool := new(MockPool)
		expectedErr := errors.New("transaction error")
		mockPool.On("Begin", mock.Anything).Return(nil, expectedErr)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.CopyRows(context.Background(), "test_table", []string{"col1"}, [][]any{{"value"}})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
		mockPool.AssertExpectations(t)
	})

	t.Run("copy error", func(t *testing.T) {
		mockPool := new(MockPool)
		mockTx := new(MockTx)

		expectedErr := errors.New("copy error")

		mockPool.On("Begin", mock.Anything).Return(mockTx, nil)
		mockTx.On("CopyFrom", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(int64(0), expectedErr)
		mockTx.On("Rollback", mock.Anything).Return(nil)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.CopyRows(context.Background(), "test_table", []string{"col1"}, [][]any{{"value"}})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to copy rows")
		mockPool.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("commit error", func(t *testing.T) {
		mockPool := new(MockPool)
		mockTx := new(MockTx)

		expectedErr := errors.New("commit error")

		mockPool.On("Begin", mock.Anything).Return(mockTx, nil)
		mockTx.On("CopyFrom", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(int64(1), nil)
		mockTx.On("Commit", mock.Anything).Return(expectedErr)
		mockTx.On("Rollback", mock.Anything).Return(nil)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.CopyRows(context.Background(), "test_table", []string{"col1"}, [][]any{{"value"}})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to commit transaction")
		mockPool.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("not all rows copied", func(t *testing.T) {
		mockPool := new(MockPool)
		mockTx := new(MockTx)

		mockPool.On("Begin", mock.Anything).Return(mockTx, nil)
		mockTx.On("CopyFrom", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(int64(1), nil)
		mockTx.On("Commit", mock.Anything).Return(nil)
		mockTx.On("Rollback", mock.Anything).Return(nil)

		pg := createPostgreSQLWithMock(mockPool)
		err := pg.CopyRows(context.Background(), "test_table", []string{"col1"}, [][]any{{"value1"}, {"value2"}})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not all rows were copied")
		mockPool.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})
}

// captureQuery makes the pool fail every query, and returns a pointer to the
// SQL statement it was asked to execute.
func captureQuery(mockPool *MockPool) *string {
	executed := new(string)

	mockPool.On("Query", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { *executed = args.String(1) }).
		Return(nil, errors.New("query failed"))

	return executed
}

func TestPostgreSQL_LabelNames(t *testing.T) {
	t.Run("selects the label names of the series matching the predicate", func(t *testing.T) {
		mockPool := new(MockPool)
		executed := captureQuery(mockPool)

		pg := createPostgreSQLWithMock(mockPool)
		_, _, err := pg.LabelNames(context.Background(), "l.metric_name = 'up'")

		assert.Error(t, err)
		assert.Contains(t, *executed, "FROM metric_labels l")
		assert.Contains(t, *executed, "WHERE (l.metric_name = 'up')")
		mockPool.AssertExpectations(t)
	})

	t.Run("reports whether a series matched at all", func(t *testing.T) {
		mockPool := new(MockPool)
		executed := captureQuery(mockPool)

		pg := createPostgreSQLWithMock(mockPool)
		_, _, err := pg.LabelNames(context.Background(), "TRUE")

		assert.Error(t, err)
		// A matching series with no label of its own still comes back, as a
		// row with no key, which is how one query answers both questions.
		assert.Contains(t, *executed, "LEFT JOIN LATERAL")
		mockPool.AssertExpectations(t)
	})

	t.Run("a key with no value of its own is not a label name", func(t *testing.T) {
		mockPool := new(MockPool)
		executed := captureQuery(mockPool)

		pg := createPostgreSQLWithMock(mockPool)
		_, _, err := pg.LabelNames(context.Background(), "TRUE")

		assert.Error(t, err)
		// The same rule as LabelValues, so a name always resolves to a value.
		assert.Contains(t, *executed, `COALESCE(e.value, '') <> ''`)
		// jsonb_each_text refuses anything but an object, which would fail
		// every metadata request over one malformed row.
		assert.Contains(t, *executed, `jsonb_typeof(l.metric_labels) = 'object'`)
		mockPool.AssertExpectations(t)
	})

	t.Run("reads the labels column alone", func(t *testing.T) {
		mockPool := new(MockPool)
		executed := captureQuery(mockPool)

		pg := createPostgreSQLWithMock(mockPool)
		_, _, err := pg.LabelNames(context.Background(), "TRUE")

		assert.Error(t, err)
		// The metric name lives in its own column and is reported by the
		// StoreAPI server, not merged into the JSONB keys here.
		assert.NotContains(t, *executed, "__name__")
		// The server sorts the names it merges, so sorting them here too would
		// be work thrown away.
		assert.NotContains(t, *executed, "ORDER BY")
		mockPool.AssertExpectations(t)
	})
}

func TestPostgreSQL_SeriesExist(t *testing.T) {
	t.Run("probes without scanning the whole table", func(t *testing.T) {
		mockPool := new(MockPool)
		executed := captureQuery(mockPool)

		pg := createPostgreSQLWithMock(mockPool)
		_, err := pg.SeriesExist(context.Background(), "l.metric_name = 'up'")

		assert.Error(t, err)
		assert.Contains(t, *executed, "SELECT EXISTS")
		assert.Contains(t, *executed, "FROM metric_labels l")
		assert.Contains(t, *executed, "WHERE (l.metric_name = 'up')")
		assert.NotContains(t, *executed, "jsonb_object_keys")
		assert.NotContains(t, *executed, "ORDER BY")
		mockPool.AssertExpectations(t)
	})
}

func TestPostgreSQL_LabelValues(t *testing.T) {
	t.Run("selects the values of the series matching the predicate", func(t *testing.T) {
		mockPool := new(MockPool)
		executed := captureQuery(mockPool)

		pg := createPostgreSQLWithMock(mockPool)
		_, err := pg.LabelValues(context.Background(), "instance", "l.metric_name = 'up'")

		assert.Error(t, err)
		assert.Contains(t, *executed, "FROM metric_labels l")
		assert.Contains(t, *executed, "l.metric_labels ? $1")
		// A key holding a JSON null reads as SQL NULL, which the string row
		// scanner cannot take, and an empty value is how a label is absent.
		assert.Contains(t, *executed, `COALESCE(l.metric_labels->>$1, '') <> ''`)
		assert.Contains(t, *executed, "AND (l.metric_name = 'up')")
		mockPool.AssertExpectations(t)
	})

	t.Run("the metric name reads the metric_name column", func(t *testing.T) {
		mockPool := new(MockPool)
		executed := captureQuery(mockPool)

		pg := createPostgreSQLWithMock(mockPool)
		_, err := pg.LabelValues(context.Background(), "__name__", "l.metric_labels @> '{\"job\":\"a\"}'")

		assert.Error(t, err)
		assert.Contains(t, *executed, "SELECT DISTINCT l.metric_name")
		assert.Contains(t, *executed, "AND (l.metric_labels @> '{\"job\":\"a\"}')")
		mockPool.AssertExpectations(t)
	})
}
