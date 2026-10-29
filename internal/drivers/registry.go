package drivers

import (
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/sijms/go-ora/v2"
	"go-db-viewer/internal/core"
)

func Registry() []core.Driver {
	return []core.Driver{
		SQLDriver{Kind: core.Postgres, DriverName: "pgx"},
		SQLDriver{Kind: core.Oracle, DriverName: "oracle"},
		SQLDriver{Kind: core.SQLServer, DriverName: "sqlserver"},
		MongoDriver{},
		IgniteDriver{Kind: core.Ignite2},
		IgniteDriver{Kind: core.Ignite3},
	}
}
