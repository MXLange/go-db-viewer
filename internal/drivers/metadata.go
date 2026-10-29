package drivers

import (
	"context"
	"strings"

	"go-db-viewer/internal/core"
)

// odbcSchema lê atributos sem interpretar delimitadores dentro de valores com chaves.
func odbcSchema(dsn string) string {
	namedDSN := false
	for len(dsn) > 0 {
		key, rest, ok := strings.Cut(dsn, "=")
		if !ok {
			break
		}
		dsn = strings.TrimLeft(rest, " \t")
		if strings.EqualFold(strings.TrimSpace(key), "DSN") {
			namedDSN = true
		}
		var value strings.Builder
		if strings.HasPrefix(dsn, "{") {
			dsn = dsn[1:]
			for len(dsn) > 0 {
				c := dsn[0]
				dsn = dsn[1:]
				if c == '}' {
					if strings.HasPrefix(dsn, "}") {
						dsn = dsn[1:]
					} else {
						break
					}
				}
				value.WriteByte(c)
			}
			_, dsn, _ = strings.Cut(dsn, ";")
		} else {
			v, tail, _ := strings.Cut(dsn, ";")
			value.WriteString(strings.TrimSpace(v))
			dsn = tail
		}
		if strings.EqualFold(strings.TrimSpace(key), "SCHEMA") {
			return value.String()
		}
	}
	if namedDSN {
		return ""
	}
	return "PUBLIC"
}

func (s *SQLSession) ResolveTable(ctx context.Context, ref core.TableReference) (core.Node, error) {
	normalize := func(value string, quoted bool) string {
		if quoted {
			return value
		}
		if s.Kind == core.Postgres {
			return strings.ToLower(value)
		}
		if s.Kind == core.SQLServer {
			return value
		}
		return strings.ToUpper(value)
	}
	schema, name := normalize(ref.Schema, ref.SchemaQuoted), normalize(ref.Name, ref.NameQuoted)
	var query string
	var args []any
	switch s.Kind {
	case core.Postgres:
		identifier := s.quote(name)
		if schema != "" {
			identifier = s.quote(schema) + "." + identifier
		}
		query = `SELECT n.nspname,c.relname,current_database() FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=pg_catalog.to_regclass($1) AND c.relkind IN ('r','p','v','m','f')`
		args = []any{identifier}
	case core.SQLServer:
		identifier := s.quote(name)
		if schema != "" {
			identifier = s.quote(schema) + "." + identifier
		}
		query = `SELECT s.name,o.name,DB_NAME() FROM sys.objects o JOIN sys.schemas s ON s.schema_id=o.schema_id WHERE o.object_id=OBJECT_ID(@p1) AND o.type IN ('U','V')`
		args = []any{identifier}
	case core.Oracle:
		query = `SELECT owner,object_name,SYS_CONTEXT('USERENV','DB_NAME') FROM all_objects WHERE owner=COALESCE(:1,SYS_CONTEXT('USERENV','CURRENT_SCHEMA')) AND object_name=:2 AND object_type IN ('TABLE','VIEW')`
		args = []any{schema, name}
	default:
		if schema == "" {
			schema = s.Schema
		}
		if schema == "" {
			return core.Node{}, core.SafeFault("Informe o schema da tabela ou SCHEMA na string ODBC.")
		}
		query = `SELECT schema_name,table_name FROM sys.tables WHERE schema_name=? AND table_name=?`
		if s.Kind == core.Ignite3 {
			query = `SELECT schema,name FROM system.tables WHERE schema=? AND name=?`
		}
		args = []any{schema, name}
	}
	r, err := s.query(ctx, query, 2, args...)
	if err != nil {
		return core.Node{}, err
	}
	if len(r.Sets) != 1 || len(r.Sets[0].Rows) != 1 || r.Truncated {
		return core.Node{}, core.SafeFault("Tabela não encontrada ou referência ambígua. Informe o schema.")
	}
	row := r.Sets[0].Rows[0]
	if len(row) < 2 {
		return core.Node{}, core.SafeFault("Resposta inválida do catálogo.")
	}
	n := core.Node{Schema: row[0].Text, Name: row[1].Text, Kind: "table"}
	if len(row) > 2 {
		n.Database = row[2].Text
	}
	return n, nil
}

func (s *SQLSession) metadata(ctx context.Context, query string, n core.Node) core.MetadataSection {
	if query == "" {
		return core.MetadataSection{Status: "unsupported", Message: "O driver não expõe estes metadados."}
	}
	r, err := s.query(ctx, query, 10000, n.Schema, n.Name)
	if err != nil || r.Truncated || len(r.Sets) != 1 {
		return core.MetadataSection{Status: "unavailable", Message: "Metadados indisponíveis. Verifique as permissões e a versão do banco."}
	}
	size := 0
	for _, row := range r.Sets[0].Rows {
		for _, cell := range row {
			size += 64 + 6*len(cell.Text)
		}
	}
	if size > core.MaxResultBytes/4 {
		return core.MetadataSection{Status: "unavailable", Message: "Metadados excedem o limite de tamanho desta seção."}
	}
	section := core.MetadataSection{Status: "available", Columns: []string{}, Rows: r.Sets[0].Rows}
	for _, c := range r.Sets[0].Columns {
		section.Columns = append(section.Columns, c.Name)
	}
	return section
}

func (s *SQLSession) GetTableProperties(ctx context.Context, n core.Node) (core.TableProperties, error) {
	var columns, pk, fk, indexes string
	switch s.Kind {
	case core.Postgres:
		columns = `SELECT a.attname AS "Coluna",pg_catalog.format_type(a.atttypid,a.atttypmod) AS "Tipo",NOT a.attnotnull AS "Aceita NULL",pg_catalog.pg_get_expr(d.adbin,d.adrelid) AS "Default" FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class t ON t.oid=a.attrelid JOIN pg_catalog.pg_namespace s ON s.oid=t.relnamespace LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum WHERE s.nspname=$1 AND t.relname=$2 AND a.attnum>0 AND NOT a.attisdropped ORDER BY a.attnum`
		pk = `SELECT c.conname AS "Chave",pg_catalog.pg_get_constraintdef(c.oid) AS "Definição" FROM pg_catalog.pg_constraint c JOIN pg_catalog.pg_class t ON t.oid=c.conrelid JOIN pg_catalog.pg_namespace s ON s.oid=t.relnamespace WHERE s.nspname=$1 AND t.relname=$2 AND c.contype='p' ORDER BY c.conname`
		fk = strings.Replace(pk, "c.contype='p'", "c.contype='f'", 1)
		indexes = `SELECT i.relname AS "Índice",x.indisunique AS "Único",pg_catalog.pg_get_indexdef(i.oid) AS "Definição" FROM pg_catalog.pg_index x JOIN pg_catalog.pg_class t ON t.oid=x.indrelid JOIN pg_catalog.pg_namespace s ON s.oid=t.relnamespace JOIN pg_catalog.pg_class i ON i.oid=x.indexrelid WHERE s.nspname=$1 AND t.relname=$2 ORDER BY i.relname`
	case core.Oracle:
		columns = `SELECT column_name AS "Coluna",data_type AS "Tipo",nullable AS "Aceita NULL",data_default AS "Default",char_length AS "Tamanho",data_precision AS "Precisão",data_scale AS "Escala" FROM all_tab_columns WHERE owner=:1 AND table_name=:2 ORDER BY column_id`
		pk = `SELECT c.constraint_name AS "Chave",k.column_name AS "Coluna",k.position AS "Posição" FROM all_constraints c JOIN all_cons_columns k ON k.owner=c.owner AND k.constraint_name=c.constraint_name WHERE c.owner=:1 AND c.table_name=:2 AND c.constraint_type='P' ORDER BY c.constraint_name,k.position`
		fk = `SELECT c.constraint_name AS "Chave",k.column_name AS "Coluna",r.owner AS "Schema destino",r.table_name AS "Tabela destino",rc.column_name AS "Coluna destino",c.delete_rule AS "ON DELETE",k.position AS "Posição" FROM all_constraints c JOIN all_cons_columns k ON k.owner=c.owner AND k.constraint_name=c.constraint_name JOIN all_constraints r ON r.owner=c.r_owner AND r.constraint_name=c.r_constraint_name JOIN all_cons_columns rc ON rc.owner=r.owner AND rc.constraint_name=r.constraint_name AND rc.position=k.position WHERE c.owner=:1 AND c.table_name=:2 AND c.constraint_type='R' ORDER BY c.constraint_name,k.position`
		indexes = `SELECT i.index_name AS "Índice",i.uniqueness AS "Unicidade",c.column_name AS "Coluna",c.column_position AS "Posição",c.descend AS "Ordem" FROM all_indexes i JOIN all_ind_columns c ON c.index_owner=i.owner AND c.index_name=i.index_name WHERE i.table_owner=:1 AND i.table_name=:2 ORDER BY i.index_name,c.column_position`
	case core.SQLServer:
		columns = `SELECT c.name AS [Coluna],t.name AS [Tipo],c.is_nullable AS [Aceita NULL],d.definition AS [Default],c.max_length AS [Bytes],c.precision AS [Precisão],c.scale AS [Escala] FROM sys.columns c JOIN sys.types t ON t.user_type_id=c.user_type_id JOIN sys.objects o ON o.object_id=c.object_id JOIN sys.schemas s ON s.schema_id=o.schema_id LEFT JOIN sys.default_constraints d ON d.object_id=c.default_object_id WHERE s.name=@p1 AND o.name=@p2 ORDER BY c.column_id`
		pk = `SELECT k.name AS [Chave],c.name AS [Coluna],ic.key_ordinal AS [Posição] FROM sys.key_constraints k JOIN sys.objects o ON o.object_id=k.parent_object_id JOIN sys.schemas s ON s.schema_id=o.schema_id JOIN sys.index_columns ic ON ic.object_id=o.object_id AND ic.index_id=k.unique_index_id JOIN sys.columns c ON c.object_id=ic.object_id AND c.column_id=ic.column_id WHERE s.name=@p1 AND o.name=@p2 AND k.type='PK' AND ic.key_ordinal>0 ORDER BY k.name,ic.key_ordinal`
		fk = `SELECT f.name AS [Chave],c.name AS [Coluna],rs.name AS [Schema destino],rt.name AS [Tabela destino],rc.name AS [Coluna destino],f.update_referential_action_desc AS [ON UPDATE],f.delete_referential_action_desc AS [ON DELETE],fc.constraint_column_id AS [Posição] FROM sys.foreign_keys f JOIN sys.foreign_key_columns fc ON fc.constraint_object_id=f.object_id JOIN sys.objects o ON o.object_id=f.parent_object_id JOIN sys.schemas s ON s.schema_id=o.schema_id JOIN sys.columns c ON c.object_id=o.object_id AND c.column_id=fc.parent_column_id JOIN sys.tables rt ON rt.object_id=fc.referenced_object_id JOIN sys.schemas rs ON rs.schema_id=rt.schema_id JOIN sys.columns rc ON rc.object_id=rt.object_id AND rc.column_id=fc.referenced_column_id WHERE s.name=@p1 AND o.name=@p2 ORDER BY f.name,fc.constraint_column_id`
		indexes = `SELECT i.name AS [Índice],i.is_unique AS [Único],c.name AS [Coluna],ic.key_ordinal AS [Posição],ic.is_descending_key AS [Descendente],ic.is_included_column AS [Incluída],i.filter_definition AS [Filtro] FROM sys.indexes i JOIN sys.objects o ON o.object_id=i.object_id JOIN sys.schemas s ON s.schema_id=o.schema_id JOIN sys.index_columns ic ON ic.object_id=i.object_id AND ic.index_id=i.index_id JOIN sys.columns c ON c.object_id=ic.object_id AND c.column_id=ic.column_id WHERE s.name=@p1 AND o.name=@p2 AND i.index_id>0 ORDER BY i.name,ic.key_ordinal,ic.index_column_id`
	case core.Ignite2:
		columns = `SELECT column_name,type,nullable,default_value,"PRECISION","SCALE" FROM sys.table_columns WHERE schema_name=? AND table_name=? ORDER BY column_name`
		pk = `SELECT index_name,"COLUMNS" FROM sys.indexes WHERE schema_name=? AND table_name=? AND is_pk=true ORDER BY index_name`
		indexes = `SELECT index_name,is_unique,index_type,"COLUMNS" FROM sys.indexes WHERE schema_name=? AND table_name=? ORDER BY index_name`
	case core.Ignite3:
		columns = `SELECT column_name,column_type,is_nullable_column,column_precision,column_scale,column_length FROM system.table_columns WHERE schema_name=? AND table_name=? ORDER BY column_ordinal`
		pk = `SELECT column_name,pk_column_ordinal FROM system.table_columns WHERE schema_name=? AND table_name=? AND pk_column_ordinal IS NOT NULL ORDER BY pk_column_ordinal`
		indexes = `SELECT index_name,is_unique_index,index_type,index_columns FROM system.indexes WHERE schema_name=? AND table_name=? ORDER BY index_name`
	}
	p := core.TableProperties{Table: n}
	p.Columns = s.metadata(ctx, columns, n)
	p.PrimaryKey = s.metadata(ctx, pk, n)
	p.ForeignKeys = s.metadata(ctx, fk, n)
	p.Indexes = s.metadata(ctx, indexes, n)
	return p, ctx.Err()
}
