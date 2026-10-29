//go:build !odbc && !windows

package drivers

func odbcAvailable(string) bool { return false }

const odbcBuilt = false
