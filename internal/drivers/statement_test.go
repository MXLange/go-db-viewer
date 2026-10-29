package drivers

import (
	"testing"

	"go-db-viewer/internal/core"
)

func TestAnalyzeStatement(t *testing.T) {
	tests := []struct {
		name string
		text string
		kind core.Kind
		rows bool
	}{
		{"select", "SELECT * FROM clientes", core.Postgres, true},
		{"comment", "/* CREATE TABLE t(id INT); */ -- UPDATE t\n SELECT 1;", core.Postgres, true},
		{"literal", "SELECT 'DELETE FROM t; RETURNING', $$INSERT;$$", core.Postgres, true},
		{"insert", "INSERT INTO t VALUES (1)", core.Postgres, false},
		{"update", "UPDATE t SET label='RETURNING'", core.Postgres, false},
		{"quoted returning", `UPDATE t SET "RETURNING"=1`, core.Postgres, false},
		{"nested returning", `UPDATE t SET x=(SELECT "RETURNING" FROM a)`, core.Postgres, false},
		{"insert returning", "INSERT INTO t VALUES (1) RETURNING id", core.Postgres, true},
		{"update returning", "UPDATE t SET x=1 RETURNING *", core.Postgres, true},
		{"delete returning", "DELETE FROM t RETURNING id", core.Postgres, true},
		{"create", "CREATE TABLE t(id INT)", core.Postgres, false},
		{"drop", "DROP TABLE t", core.Postgres, false},
		{"explain", "EXPLAIN ANALYZE DELETE FROM t", core.Postgres, true},
		{"show", "SHOW server_version", core.Postgres, true},
		{"values", "VALUES (1),(2)", core.Postgres, true},
		{"select into", "SELECT * INTO new_table FROM source", core.SQLServer, false},
		{"cte select", "WITH t AS (SELECT 1) SELECT * FROM t", core.Postgres, true},
		{"cte update", "WITH t AS (SELECT 1) UPDATE a SET id=1", core.Postgres, false},
		{"cte returning", "WITH t AS (SELECT 1) UPDATE a SET id=1 RETURNING id", core.Postgres, true},
		{"cte write inner", "WITH t AS (DELETE FROM a RETURNING id) SELECT * FROM t", core.Postgres, true},
		{"recursive cte", "WITH RECURSIVE t(id) AS (VALUES (1) UNION ALL SELECT id+1 FROM t WHERE id<10) SELECT * FROM t", core.Postgres, true},
		{"multiple ctes", `WITH "select" AS (SELECT 1), a(id) AS NOT MATERIALIZED (SELECT 2), b AS MATERIALIZED (SELECT 3) DELETE FROM t`, core.Postgres, false},
		{"sqlserver output", "INSERT INTO t OUTPUT inserted.id VALUES (1)", core.SQLServer, true},
		{"sqlserver output into", "INSERT INTO t OUTPUT inserted.id INTO audit SELECT id FROM source", core.SQLServer, false},
		{"sqlserver update output", "UPDATE t SET x=1 OUTPUT inserted.id", core.SQLServer, true},
		{"sqlserver literal output", "UPDATE t SET x='OUTPUT'", core.SQLServer, false},
		{"oracle insert", "INSERT INTO t VALUES (1); -- fim", core.Oracle, false},
		{"oracle returning into", "UPDATE t SET x=1 RETURNING id INTO :id", core.Oracle, false},
		{"oracle literal", "SELECT q'[a'; DELETE FROM t;]' FROM DUAL;", core.Oracle, true},
		{"postgres escape", `SELECT E'a\'; DELETE FROM t;'`, core.Postgres, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := analyzeStatement(tt.text, tt.kind)
			if err != nil {
				t.Fatal(err)
			}
			if s.returnsRows != tt.rows {
				t.Fatalf("retorno=%v esperado=%v", s.returnsRows, tt.rows)
			}
		})
	}
}

func TestAnalyzeRejectsScriptsBeforeExecution(t *testing.T) {
	for _, text := range []string{
		"SELECT 1; DELETE FROM t", "CREATE TABLE t(id INT); INSERT INTO t VALUES(1);", "SELECT 1;;", "/* comentário */", "BEGIN", "ROLLBACK", "SET AUTOCOMMIT=0",
		"WITH t AS (SELECT 1)", "WITH t AS SELECT 1 SELECT * FROM t", "SELECT 'unclosed", "SELECT (1", "SELECT 1)", "SELECT q'[unclosed'", "EXEC procedure_name",
	} {
		if _, err := analyzeStatement(text, core.Postgres); err == nil {
			t.Errorf("aceitou %q", text)
		}
	}
}

func TestOracleRemovesOnlyStatementTerminator(t *testing.T) {
	s, err := analyzeStatement("SELECT ';' FROM DUAL; -- fim", core.Oracle)
	if err != nil || s.text != "SELECT ';' FROM DUAL -- fim" {
		t.Fatalf("texto=%q erro=%v", s.text, err)
	}
	s, err = analyzeStatement("SELECT ';';", core.Postgres)
	if err != nil || s.text != "SELECT ';';" {
		t.Fatal("alterou o SQL PostgreSQL")
	}
}
