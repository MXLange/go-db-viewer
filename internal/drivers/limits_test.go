package drivers

import (
	"database/sql/driver"
	"io"
	"strconv"
	"strings"
	"testing"

	"go-db-viewer/internal/core"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestSQLRequestedRowLimits(t *testing.T) {
	for _, limit := range []int{1, 200, 1000, 100000} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			values := make([][]driver.Value, limit+1)
			for i := range values {
				values[i] = []driver.Value{"x"}
			}
			rows := &testRows{values: values}
			conn := &testConn{rows: rows}
			s := sqlTestSession(t, conn)
			result, err := s.Execute(t.Context(), core.Request{Text: "SELECT value FROM data", RowLimit: limit})
			if err != nil || len(result.Sets[0].Rows) != limit || result.RowLimit != limit || result.LimitReason != "rows" || !result.HasMore || !rows.closed || conn.queryCalls != 1 {
				t.Fatalf("limite=%d resultado=%+v err=%v", limit, result, err)
			}
		})
	}
	for _, count := range []int{0, 37, 200} {
		values := make([][]driver.Value, count)
		for i := range values {
			values[i] = []driver.Value{int64(i)}
		}
		s := sqlTestSession(t, &testConn{rows: &testRows{values: values}})
		result, err := s.Execute(t.Context(), core.Request{Text: "SELECT value FROM data"})
		if err != nil || result.RowLimit != 200 || len(result.Sets[0].Rows) != count || result.HasMore || result.Truncated {
			t.Fatal("zero, abaixo ou exatamente no limite informado incorretamente")
		}
	}
}

type multipleResultRows struct {
	set, index int
	closed     bool
}

func (*multipleResultRows) Columns() []string { return []string{"value"} }
func (r *multipleResultRows) Close() error    { r.closed = true; return nil }
func (r *multipleResultRows) Next(values []driver.Value) error {
	if r.index >= 2 {
		return io.EOF
	}
	values[0] = int64(r.index)
	r.index++
	return nil
}
func (r *multipleResultRows) HasNextResultSet() bool { return r.set < 1 }
func (r *multipleResultRows) NextResultSet() error {
	if !r.HasNextResultSet() {
		return io.EOF
	}
	r.set++
	r.index = 0
	return nil
}
func TestSQLSharesLimitAcrossResultSets(t *testing.T) {
	rows := &multipleResultRows{}
	conn := &testConn{query: func(string, []driver.NamedValue) (driver.Rows, error) { return rows, nil }}
	s := sqlTestSession(t, conn)
	result, err := s.Execute(t.Context(), core.Request{Text: "SELECT 1", RowLimit: 3})
	if err != nil || len(result.Sets) != 2 || len(result.Sets[0].Rows) != 2 || len(result.Sets[1].Rows) != 1 || result.LimitReason != "rows" || !rows.closed {
		t.Fatalf("limite entre resultados: %+v %v", result, err)
	}
}
func TestRowLimitDoesNotRewriteSQLOrRestrictAffectedRows(t *testing.T) {
	text := "INSERT INTO data SELECT * FROM source RETURNING id"
	conn := &testConn{query: func(query string, _ []driver.NamedValue) (driver.Rows, error) {
		if query != text {
			t.Fatal("reescreveu comando SQL")
		}
		return &testRows{values: [][]driver.Value{{int64(1)}, {int64(2)}}}, nil
	}}
	s := sqlTestSession(t, conn)
	result, err := s.Execute(t.Context(), core.Request{Text: text, RowLimit: 1})
	if err != nil || result.RowLimit != 1 || conn.queryCalls != 1 || conn.execCalls != 0 {
		t.Fatal("repetiu comando com retorno")
	}
	result, err = s.Execute(t.Context(), core.Request{Text: "UPDATE data SET value=1", RowLimit: 1})
	if err != nil || result.RowLimit != 0 || result.Affected != "7" || conn.execCalls != 1 {
		t.Fatal("limite alterou linhas afetadas")
	}
	calls := conn.calls
	if _, err = s.Execute(t.Context(), core.Request{Text: "DELETE FROM data", RowLimit: -1}); err == nil || conn.calls != calls {
		t.Fatal("limite inválido executado")
	}
}
func TestPreviewUsesRequestedPageSizeWithoutOverflow(t *testing.T) {
	for _, kind := range []core.Kind{core.Postgres, core.Oracle, core.SQLServer, core.Ignite2, core.Ignite3} {
		t.Run(string(kind), func(t *testing.T) {
			conn := &testConn{}
			s := sqlTestSession(t, conn)
			s.Kind = kind
			conn.query = func(query string, _ []driver.NamedValue) (driver.Rows, error) {
				if conn.queryCalls == 1 {
					return &testRows{values: [][]driver.Value{{"id"}}}, nil
				}
				if !strings.Contains(query, "10000000000") || !strings.Contains(query, "100001") {
					t.Fatalf("paginação: %s", query)
				}
				return &testRows{}, nil
			}
			result, err := s.Preview(t.Context(), core.PreviewRequest{Node: core.Node{Schema: "public", Name: "data", Kind: "table"}, Page: 100000, RowLimit: 100000})
			if err != nil || result.RowLimit != 100000 || conn.queryCalls != 2 {
				t.Fatalf("visualização: %+v %v", result, err)
			}
		})
	}
}
func TestMongoCursorUsesConfiguredItemLimit(t *testing.T) {
	for _, limit := range []int{1, 200, 1000} {
		for _, extra := range []int{0, 1} {
			t.Run(strconv.Itoa(limit)+"-"+strconv.Itoa(extra), func(t *testing.T) {
				docs := make([]any, limit+extra)
				for i := range docs {
					docs[i] = bson.D{{Key: "value", Value: i}}
				}
				cursor, err := mongo.NewCursorFromDocuments(docs, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				result, err := mongoDocuments(t.Context(), cursor, limit)
				if err != nil || len(result.Documents) != limit || result.RowLimit != limit || result.HasMore != (extra == 1) {
					t.Fatal("limite MongoDB incorreto")
				}
				if extra == 1 && result.LimitReason != "rows" {
					t.Fatal("motivo de truncamento ausente")
				}
			})
		}
	}
	cursor, err := mongo.NewCursorFromDocuments([]any{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := mongoDocuments(t.Context(), cursor, 200)
	if err != nil || result.RowLimit != 200 || len(result.Documents) != 0 {
		t.Fatal("consulta vazia sem limite efetivo")
	}
}
