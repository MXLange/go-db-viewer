package core

import "context"

const (
	MaxCellBytes   = 65536
	MaxResultBytes = 8 << 20
)

type Kind string

const (
	Postgres  Kind = "postgresql"
	Oracle    Kind = "oracle"
	SQLServer Kind = "sqlserver"
	MongoDB   Kind = "mongodb"
	Ignite2   Kind = "ignite2"
	Ignite3   Kind = "ignite3"
)

type DriverInfo struct {
	Kind     Kind   `json:"kind"`
	Name     string `json:"name"`
	Example  string `json:"example"`
	Language string `json:"language"`
}

type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      Kind   `json:"kind"`
	HasSecret bool   `json:"hasSecret"`
}

type ConnectionInput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	DSN  string `json:"dsn"`
}

type Node struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Database string `json:"database"`
	Schema   string `json:"schema"`
}

type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	InsertText string `json:"insertText,omitempty"`
}

type TableSuggestion struct {
	Schema     string `json:"schema"`
	Name       string `json:"name"`
	InsertText string `json:"insertText"`
}

// Cell usa texto para preservar inteiros e decimais no JavaScript.
type Cell struct {
	Text      string `json:"text"`
	Null      bool   `json:"null"`
	Truncated bool   `json:"truncated"`
}

type ResultSet struct {
	Columns []Column `json:"columns"`
	Rows    [][]Cell `json:"rows"`
}

type Result struct {
	RowLimit    int         `json:"rowLimit"`
	LimitReason string      `json:"limitReason"`
	Sets        []ResultSet `json:"sets"`
	Documents   []string    `json:"documents"`
	Affected    string      `json:"affected"`
	ElapsedMS   int64       `json:"elapsedMS"`
	Truncated   bool        `json:"truncated"`
	HasMore     bool        `json:"hasMore"`
	Message     string      `json:"message"`
}

type Request struct {
	RowLimit int    `json:"rowLimit"`
	Database string `json:"database"`
	Text     string `json:"text"`
}

type PreviewRequest struct {
	RowLimit int  `json:"rowLimit"`
	Node     Node `json:"node"`
	Page     int  `json:"page"`
}

type Driver interface {
	Info() DriverInfo
	Validate(string) error
	Open(context.Context, string) (Session, error)
}

type Session interface {
	Ping(context.Context) error
	List(context.Context, Node) ([]Node, error)
	ListTables(context.Context) ([]TableSuggestion, error)
	Describe(context.Context, Node) ([]Column, error)
	ResolveTable(context.Context, TableReference) (Node, error)
	GetTableProperties(context.Context, Node) (TableProperties, error)
	Preview(context.Context, PreviewRequest) (Result, error)
	Execute(context.Context, Request) (Result, error)
	Close() error
}

// TableReference preserva a distinção entre identificadores SQL delimitados e comuns.
type TableReference struct {
	Schema       string `json:"schema"`
	Name         string `json:"name"`
	SchemaQuoted bool   `json:"schemaQuoted"`
	NameQuoted   bool   `json:"nameQuoted"`
}

type MetadataSection struct {
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Columns []string `json:"columns"`
	Rows    [][]Cell `json:"rows"`
}

type TableProperties struct {
	Table       Node            `json:"table"`
	Columns     MetadataSection `json:"columns"`
	PrimaryKey  MetadataSection `json:"primaryKey"`
	ForeignKeys MetadataSection `json:"foreignKeys"`
	Indexes     MetadataSection `json:"indexes"`
}
