//go:build odbc || windows

package drivers

import (
	"database/sql"
	_ "github.com/alexbrainman/odbc"
	"strings"
)

func odbcAvailable(dsn string) bool {
	// A ausência do driver é descoberta durante a abertura, antes de comandos.
	return strings.Contains(strings.ToLower(dsn), "driver=") && len(sql.Drivers()) > 0
}

const odbcBuilt = true
