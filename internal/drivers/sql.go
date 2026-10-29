package drivers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go-db-viewer/internal/core"
)

type SQLDriver struct {
	Kind       core.Kind
	DriverName string
}

func (d SQLDriver) Info() core.DriverInfo {
	switch d.Kind {
	case core.Postgres:
		return core.DriverInfo{Kind: d.Kind, Name: "PostgreSQL", Language: "sql", Example: "postgresql://usuario:senha@localhost:5432/banco?sslmode=require"}
	case core.Oracle:
		return core.DriverInfo{Kind: d.Kind, Name: "Oracle", Language: "sql", Example: "oracle://usuario:senha@localhost:1521/servico"}
	case core.SQLServer:
		return core.DriverInfo{Kind: d.Kind, Name: "SQL Server", Language: "sql", Example: "sqlserver://usuario:senha@localhost:1433?database=banco&encrypt=true"}
	case core.Ignite2:
		return core.DriverInfo{Kind: d.Kind, Name: "Apache Ignite 2", Language: "sql", Example: "DRIVER={Apache Ignite};ADDRESS=localhost:10800;SCHEMA=PUBLIC;USER=usuario;PASSWORD=senha;"}
	default:
		return core.DriverInfo{Kind: d.Kind, Name: "Apache Ignite 3", Language: "sql", Example: "DRIVER={Apache Ignite 3};ADDRESS=localhost:10800;SCHEMA=PUBLIC;IDENTITY=usuario;SECRET=senha;"}
	}
}

func (d SQLDriver) Validate(dsn string) error {
	if len(dsn) == 0 || len(dsn) > 65536 {
		return errors.New("Informe uma string de conexão com até 64 KiB.")
	}
	if d.Kind == core.Ignite2 || d.Kind == core.Ignite3 {
		if !strings.Contains(strings.ToLower(dsn), "driver=") {
			return errors.New("Informe uma string ODBC com DRIVER.")
		}
		return nil
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return errors.New("String de conexão inválida. Use o formato do exemplo.")
	}
	valid := u.Scheme == "postgres" || u.Scheme == "postgresql"
	if d.Kind == core.Oracle {
		valid = u.Scheme == "oracle"
	}
	if d.Kind == core.SQLServer {
		valid = u.Scheme == "sqlserver"
	}
	if !valid {
		return errors.New("O protocolo não corresponde ao banco selecionado.")
	}
	return nil
}

func (d SQLDriver) Open(ctx context.Context, dsn string) (core.Session, error) {
	db, err := sql.Open(d.DriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(5 * time.Minute)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLSession{DB: db, Conn: conn, Kind: d.Kind, Schema: odbcSchema(dsn)}, nil
}

type SQLSession struct {
	DB     *sql.DB
	Conn   *sql.Conn
	Kind   core.Kind
	Schema string
}

func (s *SQLSession) Ping(ctx context.Context) error { return s.Conn.PingContext(ctx) }
func (s *SQLSession) Close() error                   { return errors.Join(s.Conn.Close(), s.DB.Close()) }

func (s *SQLSession) bind(n int) string {
	switch s.Kind {
	case core.Postgres:
		return "$" + strconv.Itoa(n)
	case core.Oracle:
		return ":" + strconv.Itoa(n)
	case core.SQLServer:
		return "@p" + strconv.Itoa(n)
	default:
		return "?"
	}
}

func (s *SQLSession) quote(name string) string {
	if s.Kind == core.SQLServer {
		return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (s *SQLSession) table(n core.Node) string { return s.quote(n.Schema) + "." + s.quote(n.Name) }

func (s *SQLSession) ListTables(ctx context.Context) ([]core.TableSuggestion, error) {
	query := "SELECT table_schema, table_name FROM information_schema.tables ORDER BY table_schema, table_name"
	switch s.Kind {
	case core.Oracle:
		query = "SELECT owner, object_name FROM all_objects WHERE object_type IN ('TABLE','VIEW') ORDER BY owner, object_name"
	case core.Ignite2:
		query = "SELECT schema_name, table_name FROM sys.tables ORDER BY schema_name, table_name"
	case core.Ignite3:
		query = "SELECT schema, name FROM system.tables ORDER BY schema, name"
	}
	r, err := s.query(ctx, query, 10000)
	if err != nil {
		return nil, err
	}
	if r.Truncated {
		return nil, core.SafeFault("O catálogo excede o limite de tabelas ou tamanho. Reduza os objetos visíveis à conexão.")
	}
	tables := []core.TableSuggestion{}
	if len(r.Sets) == 0 {
		return tables, nil
	}
	for _, row := range r.Sets[0].Rows {
		if len(row) < 2 || row[0].Null || row[1].Null {
			continue
		}
		schema, name := row[0].Text, row[1].Text
		tables = append(tables, core.TableSuggestion{Schema: schema, Name: name, InsertText: s.quote(schema) + "." + s.quote(name)})
	}
	return tables, nil
}

func (s *SQLSession) List(ctx context.Context, parent core.Node) ([]core.Node, error) {
	var query string
	var args []any
	kind := "schema"
	if parent.Kind == "" {
		query = "SELECT current_database()"
		switch s.Kind {
		case core.Oracle:
			query = "SELECT SYS_CONTEXT('USERENV', 'DB_NAME') FROM DUAL"
		case core.SQLServer:
			query = "SELECT DB_NAME()"
		case core.Ignite2, core.Ignite3:
			return []core.Node{{Name: "Ignite", Kind: "database", Database: "Ignite"}}, nil
		}
		kind = "database"
	} else if parent.Kind == "database" {
		query = "SELECT schema_name FROM information_schema.schemata ORDER BY schema_name"
		if s.Kind == core.Oracle {
			query = "SELECT username FROM all_users ORDER BY username"
		}
		if s.Kind == core.Ignite2 {
			query = "SELECT schema_name FROM sys.schemas ORDER BY schema_name"
		}
		if s.Kind == core.Ignite3 {
			query = "SELECT DISTINCT schema FROM system.tables ORDER BY schema"
		}
	} else if parent.Kind == "schema" {
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = " + s.bind(1) + " ORDER BY table_name"
		if s.Kind == core.Oracle {
			query = "SELECT object_name FROM all_objects WHERE owner = :1 AND object_type IN ('TABLE','VIEW') ORDER BY object_name"
		}
		if s.Kind == core.Ignite2 {
			query = "SELECT table_name FROM sys.tables WHERE schema_name = ? ORDER BY table_name"
		}
		if s.Kind == core.Ignite3 {
			query = "SELECT name FROM system.tables WHERE schema = ? ORDER BY name"
		}
		args = []any{parent.Name}
		kind = "table"
	} else {
		return []core.Node{}, nil
	}
	r, err := s.query(ctx, query, 10000, args...)
	if err != nil {
		return nil, err
	}
	var out []core.Node
	if len(r.Sets) == 0 {
		return out, nil
	}
	for _, row := range r.Sets[0].Rows {
		if len(row) == 0 {
			continue
		}
		n := core.Node{Name: row[0].Text, Kind: kind, Database: parent.Database, Schema: parent.Schema}
		if kind == "database" {
			n.Database = n.Name
		}
		if kind == "schema" {
			n.Schema = n.Name
		}
		out = append(out, n)
	}
	return out, nil
}

func (s *SQLSession) Describe(ctx context.Context, n core.Node) ([]core.Column, error) {
	query := "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = " + s.bind(1) + " AND table_name = " + s.bind(2) + " ORDER BY ordinal_position"
	switch s.Kind {
	case core.Oracle:
		query = "SELECT column_name, data_type FROM all_tab_columns WHERE owner = :1 AND table_name = :2 ORDER BY column_id"
	case core.Ignite2:
		query = "SELECT column_name, type FROM sys.table_columns WHERE schema_name = ? AND table_name = ? ORDER BY column_name"
	case core.Ignite3:
		// JDBC/ODBC expõem tipos mesmo quando o catálogo varia entre versões.
		r, err := s.query(ctx, "SELECT * FROM "+s.table(n)+" WHERE 1=0", 0)
		if err != nil {
			return nil, err
		}
		if len(r.Sets) > 0 {
			columns := r.Sets[0].Columns
			for i := range columns {
				columns[i].InsertText = s.quote(columns[i].Name)
			}
			return columns, nil
		}
		return nil, nil
	}
	r, err := s.query(ctx, query, 10000, n.Schema, n.Name)
	if err != nil {
		return nil, err
	}
	var out []core.Column
	if len(r.Sets) > 0 {
		for _, row := range r.Sets[0].Rows {
			if len(row) >= 2 {
				out = append(out, core.Column{Name: row[0].Text, Type: row[1].Text, InsertText: s.quote(row[0].Text)})
			}
		}
	}
	return out, nil
}

func (s *SQLSession) primaryKeys(ctx context.Context, n core.Node) ([]string, error) {
	query := "SELECT k.column_name FROM information_schema.table_constraints t JOIN information_schema.key_column_usage k ON t.constraint_name=k.constraint_name AND t.constraint_schema=k.constraint_schema AND t.table_name=k.table_name WHERE t.constraint_type='PRIMARY KEY' AND t.table_schema=" + s.bind(1) + " AND t.table_name=" + s.bind(2) + " ORDER BY k.ordinal_position"
	if s.Kind == core.Oracle {
		query = "SELECT c.column_name FROM all_constraints t JOIN all_cons_columns c ON t.owner=c.owner AND t.constraint_name=c.constraint_name WHERE t.constraint_type='P' AND t.owner=:1 AND t.table_name=:2 ORDER BY c.position"
	}
	if s.Kind == core.Ignite2 {
		query = "SELECT column_name FROM sys.table_columns WHERE schema_name=? AND table_name=? AND pk=true ORDER BY column_name"
	}
	if s.Kind == core.Ignite3 {
		query = "SELECT column_name FROM system.table_columns WHERE schema_name=? AND table_name=? AND pk_column_ordinal IS NOT NULL ORDER BY pk_column_ordinal"
	}
	r, err := s.query(ctx, query, 1000, n.Schema, n.Name)
	if err != nil {
		return nil, err
	}
	var keys []string
	if len(r.Sets) > 0 {
		for _, row := range r.Sets[0].Rows {
			if len(row) > 0 {
				keys = append(keys, s.quote(row[0].Text))
			}
		}
	}
	return keys, nil
}

func (s *SQLSession) Preview(ctx context.Context, req core.PreviewRequest) (core.Result, error) {
	limit, err := core.ResolveRowLimit(req.RowLimit, core.DefaultRowLimit)
	if err != nil {
		return core.Result{}, err
	}
	if req.Node.Kind != "table" {
		return core.Result{}, core.SafeFault("Selecione uma tabela ou view.")
	}
	keys, err := s.primaryKeys(ctx, req.Node)
	if err != nil {
		return core.Result{}, err
	}
	order := ""
	if len(keys) > 0 {
		order = " ORDER BY " + strings.Join(keys, ", ")
	}
	offset := int64(req.Page) * int64(limit)
	query := "SELECT * FROM " + s.table(req.Node) + order
	if s.Kind == core.SQLServer || s.Kind == core.Oracle {
		if order == "" && s.Kind == core.SQLServer {
			query += " ORDER BY (SELECT NULL)"
		}
		query += fmt.Sprintf(" OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", offset, limit+1)
	} else {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit+1, offset)
	}
	r, err := s.query(ctx, query, limit)
	if len(keys) == 0 {
		r.Message = strings.TrimSpace(r.Message + " Sem chave primária disponível. A ordem entre páginas pode variar.")
	}
	return r, err
}

func (s *SQLSession) Execute(ctx context.Context, req core.Request) (core.Result, error) {
	limit, err := core.ResolveRowLimit(req.RowLimit, core.DefaultRowLimit)
	if err != nil {
		return core.Result{}, err
	}
	statement, err := analyzeStatement(req.Text, s.Kind)
	if err != nil {
		return core.Result{}, err
	}
	if statement.returnsRows {
		return s.query(ctx, statement.text, limit)
	}
	// Conn evita o retry automático de DB.ExecContext após driver.ErrBadConn.
	r, err := s.Conn.ExecContext(ctx, statement.text)
	if err != nil {
		return core.Result{}, err
	}
	affected, err := r.RowsAffected()
	if err != nil {
		return core.Result{Message: "Comando concluído. O driver não informou as linhas afetadas."}, nil
	}
	return core.Result{Affected: strconv.FormatInt(affected, 10), Message: "Comando concluído."}, nil
}

func (s *SQLSession) query(ctx context.Context, query string, limit int, args ...any) (core.Result, error) {
	rows, err := s.Conn.QueryContext(ctx, query, args...)
	if err != nil {
		return core.Result{}, err
	}
	defer rows.Close()
	result := core.Result{RowLimit: limit}
	count, size := 0, 0
	for {
		names, err := rows.Columns()
		if err != nil {
			return core.Result{}, err
		}
		types, err := rows.ColumnTypes()
		if err != nil {
			return core.Result{}, err
		}
		set := core.ResultSet{Columns: []core.Column{}, Rows: [][]core.Cell{}}
		for i, name := range names {
			set.Columns = append(set.Columns, core.Column{Name: name, Type: types[i].DatabaseTypeName()})
			size += 64 + 6*(len(name)+len(types[i].DatabaseTypeName()))
		}
		if size > core.MaxResultBytes {
			return core.Result{}, core.SafeFault("Os metadados excedem o limite de 8 MiB. Selecione menos colunas.")
		}
		for rows.Next() {
			if count >= limit {
				result.Truncated = true
				result.HasMore = true
				result.LimitReason = "rows"
				result.Sets = append(result.Sets, set)
				return result, nil
			}
			values, dest := make([]any, len(names)), make([]any, len(names))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				return core.Result{}, err
			}
			cells := make([]core.Cell, len(values))
			for i, v := range values {
				cells[i] = cell(v)
				// Reserva o pior caso de escaping JSON e os campos da célula.
				size += 64 + 6*len(cells[i].Text)
			}
			if size > core.MaxResultBytes {
				result.Truncated = true
				result.LimitReason = "bytes"
				result.Message = "Limite de tamanho atingido. Use uma consulta com menos colunas ou registros."
				result.Sets = append(result.Sets, set)
				return result, nil
			}
			set.Rows = append(set.Rows, cells)
			count++
		}
		if err := rows.Err(); err != nil {
			return core.Result{}, err
		}
		result.Sets = append(result.Sets, set)
		if !rows.NextResultSet() {
			return result, rows.Err()
		}
	}
}

func cell(value any) core.Cell {
	if value == nil {
		return core.Cell{Null: true}
	}
	var text string
	switch v := value.(type) {
	case []byte:
		if utf8.Valid(v) {
			text = string(v)
		} else {
			text = "base64:" + base64.StdEncoding.EncodeToString(v)
		}
	case time.Time:
		text = v.Format(time.RFC3339Nano)
	default:
		text = fmt.Sprint(v)
	}
	truncated := len(text) > core.MaxCellBytes
	if truncated {
		text = text[:core.MaxCellBytes]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	return core.Cell{Text: text, Truncated: truncated}
}
