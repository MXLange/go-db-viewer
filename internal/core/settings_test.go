package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestSettingsDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "settings.json")
	s := NewSettingsStore(path)
	if _, err := s.Save(DefaultSettings()); err == nil {
		t.Fatal("salvou antes de carregar")
	}
	loaded, err := s.Load()
	if err != nil || loaded.Settings != DefaultSettings() || loaded.Warning != "" {
		t.Fatalf("padrão: %+v %v", loaded, err)
	}
	want := Settings{Theme: "light", MaxRows: 1000}
	if _, err = s.Save(want); err != nil {
		t.Fatal(err)
	}
	loaded, err = NewSettingsStore(path).Load()
	if err != nil || loaded.Settings != want {
		t.Fatal("não restaurou configurações")
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("configurações sem permissão privada")
	}
}
func TestSettingsInvalidFileAndInvalidUpdates(t *testing.T) {
	for _, content := range []string{"invalid", `{"theme":"invalid","maxRows":200}`, `{"theme":"system","maxRows":0}`, `{"theme":"dark","maxRows":1.5}`} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			store := NewSettingsStore(path)
			loaded, err := store.Load()
			if err != nil || loaded.Warning == "" || loaded.Settings != DefaultSettings() {
				t.Fatal("não recuperou configurações")
			}
			backups, _ := filepath.Glob(path + ".invalid-*")
			if len(backups) != 1 {
				t.Fatal("não preservou arquivo")
			}
			b, _ := os.ReadFile(backups[0])
			if string(b) != content {
				t.Fatal("cópia alterada")
			}
			if _, err = store.Save(DefaultSettings()); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			for _, settings := range []Settings{{Theme: "invalid", MaxRows: 200}, {Theme: "dark", MaxRows: -1}, {Theme: "light", MaxRows: 100001}, {Theme: "system", MaxRows: 0}} {
				if _, err = store.Save(settings); err == nil {
					t.Fatalf("aceitou %+v", settings)
				}
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("alterou arquivo após erro")
			}
		})
	}
}
func TestSettingsFailedWritePreservesLastValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := NewSettingsStore(path)
	_, _ = store.Load()
	_, _ = store.Save(DefaultSettings())
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(Settings{Theme: "light", MaxRows: 1000}); err == nil {
		t.Fatal("ignorou falha de gravação")
	}
	loaded, _ := store.Load()
	if loaded.Settings != DefaultSettings() {
		t.Fatal("alterou memória após falha")
	}
	if _, err := NewSettingsStore(path).Load(); err == nil {
		t.Fatal("ignorou falha de leitura")
	}
}
func TestSettingsConcurrentReadsAndWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := NewSettingsStore(path)
	_, _ = store.Load()
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			_, err := store.Save(Settings{Theme: "light", MaxRows: i + 1})
			if err != nil {
				t.Error(err)
			}
			if _, err = store.Load(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	loaded, err := NewSettingsStore(path).Load()
	if err != nil || loaded.Settings.MaxRows < 1 || loaded.Settings.MaxRows > 10 {
		t.Fatal("gravação concorrente inválida")
	}
}
func TestServiceAppliesDefaultAndRequestLimit(t *testing.T) {
	store, _ := newTestStore(t)
	config := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	_, _ = config.Load()
	_, _ = config.Save(Settings{Theme: "system", MaxRows: 1000})
	conn := &fakeSession{}
	service := NewServiceWithSettings(t.Context(), store, config, &fakeDriver{sessions: []*fakeSession{conn}})
	defer service.Close()
	id, err := service.Connect(ConnectionInput{Kind: Postgres, DSN: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{0, 1, 200, 1000, 100000} {
		if _, err = service.Execute(id, Request{Text: "SELECT 1", RowLimit: limit}); err != nil {
			t.Fatal(err)
		}
		expected := limit
		if expected == 0 {
			expected = 1000
		}
		if conn.lastRequest.RowLimit != expected {
			t.Fatal("limite da consulta incorreto")
		}
		if _, err = service.Preview(id, PreviewRequest{Node: Node{Kind: "table"}, RowLimit: limit}); err != nil {
			t.Fatal(err)
		}
		if conn.lastPreview.RowLimit != expected {
			t.Fatal("limite da página incorreto")
		}
	}
	if _, err = config.Save(Settings{Theme: "dark", MaxRows: 500}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Execute(id, Request{Text: "SELECT 1"}); err != nil || conn.lastRequest.RowLimit != 500 {
		t.Fatal("consulta não acompanhou configuração atualizada")
	}
	calls := conn.calls
	for _, limit := range []int{-1, 100001} {
		if _, err = service.Execute(id, Request{Text: "DELETE FROM x", RowLimit: limit}); err == nil {
			t.Fatal("executou limite inválido")
		}
		if _, err = service.Preview(id, PreviewRequest{RowLimit: limit}); err == nil {
			t.Fatal("visualizou limite inválido")
		}
	}
	if conn.calls != calls {
		t.Fatal("limite inválido chegou ao driver")
	}
	if _, err = ResolveRowLimit(0, 200); err != nil {
		t.Fatal(err)
	}
	if _, err = ResolveRowLimit(0, 0); err == nil || !strings.Contains(err.Error(), "limite") {
		t.Fatal("aceitou padrão inválido")
	}
}
