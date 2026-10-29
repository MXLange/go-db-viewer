package drivers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json/v2"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"uuid"

	"go-db-viewer/internal/core"
)

func TestSingleStatement(t *testing.T) {
	for _, text := range []string{"SELECT ';'", "SELECT 1; -- final", "SELECT $$a;b$$;", "SELECT $body$a;b$body$;", "SELECT [a;b] FROM [x]]y]", "/* a /* b */ c */ SELECT 1", "SELECT 'a'';b';"} {
		if err := singleStatement(text); err != nil {
			t.Errorf("comando válido %q: %v", text, err)
		}
	}
	for _, text := range []string{"SELECT 1; DELETE FROM users", "SELECT 1;;", "BEGIN", "COMMIT", "ROLLBACK", "SET AUTOCOMMIT=0", "START TRANSACTION", "SELECT 'abc", "SELECT $$abc", "/* abc", "-- sem comando"} {
		if err := singleStatement(text); err == nil {
			t.Errorf("aceitou %q", text)
		}
	}
}

type testDriver struct{ conn *testConn }

func (d testDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

type testConn struct {
	query      func(string, []driver.NamedValue) (driver.Rows, error)
	rows       *testRows
	calls      int
	execErr    error
	queryErr   error
	queryCalls int
	execCalls  int
}

func (*testConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unsupported") }
func (*testConn) Close() error                        { return nil }
func (*testConn) Begin() (driver.Tx, error)           { return nil, errors.New("unsupported") }
func (c *testConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.calls++
	c.execCalls++
	return driver.RowsAffected(7), c.execErr
}
func (c *testConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.calls++
	c.queryCalls++
	if c.query != nil {
		return c.query(query, args)
	}
	return c.rows, c.queryErr
}

type testRows struct {
	names  []string
	values [][]driver.Value
	index  int
	closed bool
	err    error
}

func (r *testRows) Columns() []string {
	if r.names != nil {
		return r.names
	}
	return []string{"value"}
}
func (r *testRows) Close() error { r.closed = true; return nil }
func (r *testRows) Next(dest []driver.Value) error {
	if r.index == len(r.values) {
		if r.err != nil {
			return r.err
		}
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
func (*testRows) ColumnTypeDatabaseTypeName(int) string { return "NUMERIC" }
func sqlTestSession(t *testing.T, c *testConn) *SQLSession {
	t.Helper()
	name := "test-" + uuid.New().String()
	sql.Register(name, testDriver{conn: c})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	s := &SQLSession{DB: db, Conn: conn, Kind: core.Postgres}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBadConnectionDoesNotReplayWrite(t *testing.T) {
	c := &testConn{execErr: driver.ErrBadConn}
	s := sqlTestSession(t, c)
	_, err := s.Execute(t.Context(), core.Request{Text: "DELETE FROM demo"})
	if err == nil || c.calls != 1 {
		t.Fatalf("erro=%v execuções=%d", err, c.calls)
	}
}

func TestResultLimitClosesCursorAndPreservesNumbers(t *testing.T) {
	r := &testRows{values: [][]driver.Value{{int64(9223372036854775807)}, {[]byte("12345678901234567890.123456789")}, {nil}, {"extra"}}}
	s := sqlTestSession(t, &testConn{rows: r})
	result, err := s.query(t.Context(), "SELECT value FROM demo", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || !result.HasMore || !r.closed || len(result.Sets[0].Rows) != 3 {
		t.Fatal("limite ou fechamento incorreto")
	}
	values := result.Sets[0].Rows
	if values[0][0].Text != "9223372036854775807" || values[1][0].Text != "12345678901234567890.123456789" || !values[2][0].Null {
		t.Fatal("perdeu precisão ou NULL")
	}
}

func TestEncodedSizeLimitDoesNotSkipToNextPage(t *testing.T) {
	values := make([][]driver.Value, 100)
	for i := range values {
		values[i] = []driver.Value{strings.Repeat("\x00", core.MaxCellBytes)}
	}
	rows := &testRows{values: values}
	s := sqlTestSession(t, &testConn{rows: rows})
	r, err := s.query(t.Context(), "SELECT large_value FROM demo", 100)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Truncated || r.HasMore || len(b) > core.MaxResultBytes || !rows.closed {
		t.Fatal("limite JSON ou paginação incorreto")
	}
}

func TestResultsHandleEmptyAndFailure(t *testing.T) {
	t.Run("vazio", func(t *testing.T) {
		rows := &testRows{}
		s := sqlTestSession(t, &testConn{rows: rows})
		r, err := s.query(t.Context(), "SELECT 1 WHERE false", 1000)
		if err != nil || len(r.Sets) != 1 || len(r.Sets[0].Rows) != 0 || !rows.closed {
			t.Fatal("resultado vazio incorreto")
		}
	})
	t.Run("erro no cursor", func(t *testing.T) {
		rows := &testRows{err: errors.New("network")}
		s := sqlTestSession(t, &testConn{rows: rows})
		_, err := s.query(t.Context(), "SELECT 1", 1000)
		if err == nil || !rows.closed {
			t.Fatal("erro ignorado ou cursor aberto")
		}
	})
}

func TestCellTruncationAndBinary(t *testing.T) {
	c := cell(strings.Repeat("á", core.MaxCellBytes))
	if !c.Truncated || len(c.Text) > core.MaxCellBytes {
		t.Fatal("valor não truncado")
	}
	if c := cell([]byte{0xff, 0xfe}); !strings.HasPrefix(c.Text, "base64:") {
		t.Fatal("binário sem base64")
	}
}

func TestIdentifiersAreQuoted(t *testing.T) {
	for _, kind := range []core.Kind{core.Postgres, core.Oracle, core.SQLServer, core.Ignite2, core.Ignite3} {
		s := SQLSession{Kind: kind}
		name := `x"; DROP TABLE demo;--]`
		quoted := s.quote(name)
		if kind == core.SQLServer {
			if !strings.Contains(quoted, "]]") {
				t.Fatal("colchete sem escape")
			}
		} else if !strings.Contains(quoted, `""`) {
			t.Fatal("aspas sem escape")
		}
	}
}

func TestExecAffectedCount(t *testing.T) {
	c := &testConn{}
	s := sqlTestSession(t, c)
	r, err := s.Execute(t.Context(), core.Request{Text: "UPDATE demo SET value=1"})
	if err != nil || r.Affected != strconv.Itoa(7) || c.calls != 1 {
		t.Fatal("contagem ou execução incorreta")
	}
}

func TestAutomaticExecutionChoosesOnePath(t *testing.T) {
	for _, tt := range []struct {
		text  string
		kind  core.Kind
		query bool
	}{
		{"SELECT 1", core.Postgres, true},
		{"CREATE TABLE t(id INT)", core.Postgres, false},
		{"INSERT INTO t VALUES (1)", core.Postgres, false},
		{"WITH a AS (SELECT 1) UPDATE t SET id=2", core.Postgres, false},
		{"INSERT INTO t VALUES(1) RETURNING id", core.Postgres, true},
		{"UPDATE t SET id=2 OUTPUT inserted.id", core.SQLServer, true},
	} {
		t.Run(tt.text, func(t *testing.T) {
			c := &testConn{rows: &testRows{values: [][]driver.Value{{int64(1)}}}}
			s := sqlTestSession(t, c)
			s.Kind = tt.kind
			r, err := s.Execute(t.Context(), core.Request{Text: tt.text})
			if err != nil {
				t.Fatal(err)
			}
			if c.calls != 1 || (c.queryCalls == 1) != tt.query {
				t.Fatal("caminho ou quantidade de execuções incorreto")
			}
			if tt.query && len(r.Sets) != 1 {
				t.Fatal("resultado ausente")
			}
			if !tt.query && r.Affected != "7" {
				t.Fatal("linhas afetadas ausentes")
			}
		})
	}
}

func TestQueryFailureNeverFallsBackToExec(t *testing.T) {
	c := &testConn{queryErr: driver.ErrBadConn}
	s := sqlTestSession(t, c)
	_, err := s.Execute(t.Context(), core.Request{Text: "INSERT INTO t VALUES(1) RETURNING id"})
	if err == nil || c.queryCalls != 1 || c.execCalls != 0 {
		t.Fatal("repetiu o comando após falha")
	}
}

func TestMultipleStatementsNeverReachDriver(t *testing.T) {
	c := &testConn{}
	s := sqlTestSession(t, c)
	_, err := s.Execute(t.Context(), core.Request{Text: "CREATE TABLE t(id INT); INSERT INTO t VALUES(1);"})
	if err == nil || c.calls != 0 {
		t.Fatal("executou parte do bloco inválido")
	}
}

func TestCatalogQuotesQualifiedNamesForEachDriver(t *testing.T) {
	for _, kind := range []core.Kind{core.Postgres, core.Oracle, core.SQLServer, core.Ignite2, core.Ignite3} {
		t.Run(string(kind), func(t *testing.T) {
			rows := &testRows{names: []string{"schema", "table"}, values: [][]driver.Value{{`Sales Team`, `Order"Detail]`}, {"public", "clientes"}}}
			c := &testConn{rows: rows}
			s := sqlTestSession(t, c)
			s.Kind = kind
			tables, err := s.ListTables(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(tables) != 2 || tables[0].Schema != "Sales Team" || tables[0].Name != `Order"Detail]` || !rows.closed || c.calls != 1 {
				t.Fatal("catálogo incorreto")
			}
			want := `"Sales Team"."Order""Detail]"`
			if kind == core.SQLServer {
				want = `[Sales Team].[Order"Detail]]]`
			}
			if tables[0].InsertText != want {
				t.Fatalf("identificador=%q esperado=%q", tables[0].InsertText, want)
			}
		})
	}
}
