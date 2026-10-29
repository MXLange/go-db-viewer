package vault

import "github.com/zalando/go-keyring"

type NativeVault struct{}

func (NativeVault) Get(id string) (string, error) { return keyring.Get("go-db-viewer", id) }
func (NativeVault) Set(id, secret string) error   { return keyring.Set("go-db-viewer", id, secret) }
func (NativeVault) Delete(id string) error        { return keyring.Delete("go-db-viewer", id) }
