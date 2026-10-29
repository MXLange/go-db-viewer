package drivers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	"go-db-viewer/internal/core"
)

// Configure somente bancos de teste. A suíte cria e remove objetos próprios.
func TestIntegration(t *testing.T) {
	for _, d := range Registry() {
		t.Run(string(d.Info().Kind), func(t *testing.T) {
			dsn := os.Getenv("DBVIEWER_" + strings.ToUpper(string(d.Info().Kind)) + "_DSN")
			if dsn == "" {
				t.Skip("DSN de integração não configurada")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			if err := d.Validate(dsn); err != nil {
				t.Fatal("Formato da DSN de teste inválido")
			}
			session, err := d.Open(ctx, dsn)
			if err != nil {
				t.Fatal("Falha ao abrir banco de teste")
			}
			defer session.Close()
			if err := session.Ping(ctx); err != nil {
				t.Fatal("Falha no ping do banco de teste")
			}
			name := "dbviewer_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
			if d.Info().Kind == core.MongoDB {
				testMongoIntegration(t, ctx, session, name)
				return
			}
			idType := "BIGINT"
			if d.Info().Kind == core.Oracle {
				idType = "NUMBER(19)"
			}
			run := func(sql string) core.Result {
				t.Helper()
				r, err := session.Execute(ctx, core.Request{Text: sql})
				if err != nil {
					t.Fatal("Falha na execução SQL de integração")
				}
				return r
			}
			run(fmt.Sprintf("CREATE TABLE %s (id %s PRIMARY KEY, amount DECIMAL(30,9), label VARCHAR(100))", name, idType))
			defer func() {
				closeCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
				defer stop()
				if _, err := session.Execute(closeCtx, core.Request{Text: "DROP TABLE " + name}); err != nil {
					t.Error("Não foi possível remover a tabela de teste " + name)
				}
			}()
			run("INSERT INTO " + name + " (id, amount, label) VALUES (9223372036854775807,12345678901234567890.123456789,'ação')")
			result := run("SELECT id, amount, label FROM " + name)
			if len(result.Sets) != 1 || len(result.Sets[0].Rows) != 1 {
				t.Fatal("Resultado SQL inválido")
			}
			row := result.Sets[0].Rows[0]
			if row[0].Text != "9223372036854775807" || row[1].Text != "12345678901234567890.123456789" || row[2].Text != "ação" {
				t.Fatal("Perda de precisão ou Unicode no resultado SQL")
			}
			if d.Info().Kind == core.Postgres {
				inserted := run("INSERT INTO " + name + " (id, label) VALUES (2,'retorno') RETURNING id")
				if len(inserted.Sets) != 1 || len(inserted.Sets[0].Rows) != 1 || inserted.Sets[0].Rows[0][0].Text != "2" {
					t.Fatal("INSERT RETURNING sem resultado")
				}
				updated := run("WITH source AS (SELECT 2 AS id) UPDATE " + name + " SET label='cte' WHERE id IN (SELECT id FROM source)")
				if updated.Affected != "1" {
					t.Fatal("WITH UPDATE sem contagem")
				}
				changed := run("WITH changed AS (DELETE FROM " + name + " WHERE id=2 RETURNING id) SELECT id FROM changed")
				if len(changed.Sets) != 1 || len(changed.Sets[0].Rows) != 1 || changed.Sets[0].Rows[0][0].Text != "2" {
					t.Fatal("WITH SELECT sem resultado")
				}
				if _, err := session.Execute(ctx, core.Request{Text: "INSERT INTO " + name + " (id) VALUES(3); INSERT INTO " + name + " (id) VALUES(4);"}); err == nil {
					t.Fatal("aceitou múltiplos comandos em um bloco")
				}
				if run("SELECT COUNT(*) FROM " + name).Sets[0].Rows[0][0].Text != "1" {
					t.Fatal("executou parte do bloco rejeitado")
				}
			}
			run("UPDATE " + name + " SET label=NULL")
			if !run("SELECT label FROM " + name).Sets[0].Rows[0][0].Null {
				t.Fatal("NULL não preservado")
			}
			roots, err := session.List(ctx, core.Node{})
			if err != nil || len(roots) == 0 {
				t.Fatal("Não listou o banco")
			}
			schemas, err := session.List(ctx, roots[0])
			if err != nil {
				t.Fatal("Não listou schemas")
			}
			found := false
			for _, schema := range schemas {
				nodes, err := session.List(ctx, schema)
				if err != nil {
					t.Fatal("Não listou tabelas")
				}
				for _, node := range nodes {
					if strings.EqualFold(node.Name, name) {
						columns, err := session.Describe(ctx, node)
						if err != nil || len(columns) != 3 {
							t.Fatal("Descrição inválida")
						}
						preview, err := session.Preview(ctx, core.PreviewRequest{Node: node})
						if err != nil || len(preview.Sets) == 0 || len(preview.Sets[0].Rows) != 1 {
							t.Fatal("Visualização inválida")
						}
						found = true
					}
				}
			}
			if !found {
				t.Fatal("Tabela de teste ausente da árvore")
			}
			tables, err := session.ListTables(ctx)
			if err != nil {
				t.Fatal("Não carregou o catálogo de autocomplete")
			}
			catalogFound := false
			for _, table := range tables {
				if strings.EqualFold(table.Name, name) && table.Schema != "" && table.InsertText != "" {
					catalogFound = true
				}
			}
			if !catalogFound {
				t.Fatal("Tabela de teste ausente do catálogo de autocomplete")
			}
			node, err := session.ResolveTable(ctx, core.TableReference{Name: name})
			if err != nil || !strings.EqualFold(node.Name, name) {
				t.Fatal("Não resolveu a tabela pelo contexto da conexão")
			}
			properties, err := session.GetTableProperties(ctx, node)
			if err != nil || properties.Columns.Status != "available" || len(properties.Columns.Rows) != 3 || properties.PrimaryKey.Status != "available" || len(properties.PrimaryKey.Rows) == 0 || properties.Indexes.Status != "available" || len(properties.Indexes.Rows) == 0 {
				t.Fatal("Propriedades da tabela incompletas")
			}
			run("DELETE FROM " + name)
			if len(run("SELECT * FROM " + name).Sets[0].Rows) != 0 {
				t.Fatal("DELETE não aplicado")
			}
		})
	}
}

func testMongoIntegration(t *testing.T, ctx context.Context, s core.Session, name string) {
	t.Helper()
	db := os.Getenv("DBVIEWER_MONGODB_DATABASE")
	if db == "" {
		db = "dbviewer_test"
	}
	run := func(text string) core.Result {
		t.Helper()
		r, err := s.Execute(ctx, core.Request{Database: db, Text: text})
		if err != nil {
			t.Fatal("Falha no comando MongoDB de integração")
		}
		return r
	}
	run(fmt.Sprintf(`{"insert":%q,"documents":[{"_id":{"$numberLong":"9223372036854775807"},"amount":{"$numberDecimal":"12345678901234567890.123456789"},"nested":{"label":"ação"},"nullable":null}]}`, name))
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.Execute(closeCtx, core.Request{Database: db, Text: fmt.Sprintf(`{"drop":%q}`, name)}); err != nil {
			t.Error("Não foi possível remover a coleção de teste " + name)
		}
	}()
	r := run(fmt.Sprintf(`{"find":%q,"filter":{},"batchSize":1}`, name))
	if len(r.Documents) != 1 || !strings.Contains(r.Documents[0], "9223372036854775807") || !strings.Contains(r.Documents[0], "12345678901234567890.123456789") || !strings.Contains(r.Documents[0], "ação") {
		t.Fatal("Extended JSON perdeu tipos ou precisão")
	}
	node := core.Node{Name: name, Kind: "collection", Database: db}
	if _, err := s.Preview(ctx, core.PreviewRequest{Node: node}); err != nil {
		t.Fatal("Falha na visualização MongoDB")
	}
	if cols, err := s.Describe(ctx, node); err != nil || len(cols) == 0 {
		t.Fatal("Falha na descrição MongoDB")
	}
	run(fmt.Sprintf(`{"update":%q,"updates":[{"q":{},"u":{"$set":{"updated":true}},"multi":true}]}`, name))
	run(fmt.Sprintf(`{"delete":%q,"deletes":[{"q":{},"limit":0}]}`, name))
	if r := run(fmt.Sprintf(`{"find":%q,"filter":{}}`, name)); len(r.Documents) != 0 {
		t.Fatal("DELETE MongoDB não aplicado")
	}
}
