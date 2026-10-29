package drivers

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"go-db-viewer/internal/core"
)

func TestResolveTableUsesParametersAndIdentifierFolding(t *testing.T) {
	for _, kind := range []core.Kind{core.Postgres, core.Oracle, core.SQLServer, core.Ignite2, core.Ignite3} {
		t.Run(string(kind), func(t *testing.T) {
			c := &testConn{}
			s := sqlTestSession(t, c)
			s.Kind = kind
			s.Schema = "CUSTOM"
			c.query = func(query string, args []driver.NamedValue) (driver.Rows, error) {
				if strings.Contains(query, "attack") {
					t.Fatal("referência interpolada na consulta")
				}
				if kind == core.Postgres && args[0].Value != `"Mixed"."attack'"` {
					t.Fatalf("referência: %v", args)
				}
				if kind == core.Oracle && (args[0].Value != "Mixed" || args[1].Value != "ATTACK'") {
					t.Fatalf("fold: %v", args)
				}
				return &testRows{names: []string{"schema", "name"}, values: [][]driver.Value{{"Mixed", "attack'"}}}, nil
			}
			n, err := s.ResolveTable(t.Context(), core.TableReference{Schema: "Mixed", SchemaQuoted: true, Name: "attack'"})
			if err != nil || n.Schema != "Mixed" || n.Name != "attack'" || c.execCalls != 0 {
				t.Fatalf("node=%v error=%v", n, err)
			}
		})
	}
}
func TestResolveTableRejectsMissingAndAmbiguousObjects(t *testing.T) {
	for _, rows := range [][][]driver.Value{nil, {{"public", "a"}, {"sales", "a"}}} {
		s := sqlTestSession(t, &testConn{rows: &testRows{names: []string{"schema", "name"}, values: rows}})
		if _, err := s.ResolveTable(t.Context(), core.TableReference{Name: "a"}); err == nil {
			t.Fatal("aceitou resolução sem objeto único")
		}
	}
}
func TestPropertiesReportsPartialMetadataWithoutReadingTable(t *testing.T) {
	for _, kind := range []core.Kind{core.Postgres, core.Oracle, core.SQLServer, core.Ignite2, core.Ignite3} {
		t.Run(string(kind), func(t *testing.T) {
			c := &testConn{}
			s := sqlTestSession(t, c)
			s.Kind = kind
			c.query = func(query string, args []driver.NamedValue) (driver.Rows, error) {
				if strings.Contains(query, "private_table") {
					t.Fatal("leitura direta da tabela")
				}
				if len(args) != 2 || args[0].Value != "private_schema" || args[1].Value != "private_table" {
					t.Fatal("filtro não parametrizado")
				}
				if c.queryCalls == 2 {
					return nil, errors.New("secret credentials")
				}
				return &testRows{names: []string{"Coluna"}, values: [][]driver.Value{{"id"}}}, nil
			}
			p, err := s.GetTableProperties(t.Context(), core.Node{Schema: "private_schema", Name: "private_table"})
			if err != nil || p.Columns.Status != "available" || p.PrimaryKey.Status != "unavailable" || p.Indexes.Status != "available" {
				t.Fatalf("parcial: %+v %v", p, err)
			}
			if strings.Contains(p.PrimaryKey.Message, "secret") || c.execCalls != 0 {
				t.Fatal("mensagem externa ou escrita")
			}
			if (kind == core.Ignite2 || kind == core.Ignite3) && p.ForeignKeys.Status != "unsupported" {
				t.Fatal("FK não suportada marcada como vazia")
			}
		})
	}
}
func TestODBCSchemaIgnoresAttributesInsidePasswords(t *testing.T) {
	for dsn, want := range map[string]string{
		"DRIVER={Apache Ignite};PASSWORD={x;SCHEMA=SECRET;};SCHEMA=REAL;":  "REAL",
		"DRIVER={Apache Ignite};PASSWORD={x}};SCHEMA=SECRET;};":            "PUBLIC",
		"DRIVER={Apache Ignite};DSN=custom;":                               "",
		"DRIVER={Apache Ignite};PASSWORD= {x;SCHEMA=SECRET;};SCHEMA=REAL;": "REAL",
		"DRIVER={Apache Ignite};SCHEMA={Case;Schema};":                     "Case;Schema",
	} {
		if got := odbcSchema(dsn); got != want {
			t.Errorf("got=%q want=%q", got, want)
		}
	}
}
