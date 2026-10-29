package core

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func newWorkspaceTestStore(t *testing.T) (*WorkspaceStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config", "workspace.json")
	s := NewWorkspaceStore(path)
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	return s, path
}
func saveTestScript(t *testing.T, s *WorkspaceStore, title, profile string) SavedScript {
	t.Helper()
	script, err := s.SaveScript(SavedScript{Title: title, Text: "SELECT 'ação';", Kind: Postgres, ProfileID: profile, ConnectionName: "Local", Database: "dbviewer"})
	if err != nil {
		t.Fatal(err)
	}
	return script
}
func TestWorkspaceLibrarySurvivesClosingTabs(t *testing.T) {
	s, path := newWorkspaceTestStore(t)
	a := saveTestScript(t, s, "Clientes", "profile")
	b := saveTestScript(t, s, "Documento", "")
	state := WorkspaceState{Version: 2, ActiveTabID: 7, OpenTabs: []OpenScript{{ID: 3, ScriptID: a.ID, Anchor: 2, Head: 10}, {ID: 7, ScriptID: b.ID}}}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewWorkspaceStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workspace.ActiveTabID != 7 || loaded.Workspace.OpenTabs[0].Head != 10 || loaded.Workspace.Scripts[0].Text != a.Text {
		t.Fatal("estado restaurado incorreto")
	}
	if err := s.Save(WorkspaceState{Version: 2}); err != nil {
		t.Fatal(err)
	}
	loaded, err = NewWorkspaceStore(path).Load()
	if err != nil || len(loaded.Workspace.OpenTabs) != 0 || len(loaded.Workspace.Scripts) != 2 {
		t.Fatal("fechar abas apagou a biblioteca")
	}
	content, _ := os.ReadFile(path)
	for _, forbidden := range []string{"sessionID", "dsn", "password", "result", "busy", "error"} {
		if strings.Contains(string(content), `"`+forbidden+`"`) {
			t.Fatal("campo transitório salvo")
		}
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("permissões incorretas")
	}
}
func TestScriptNamesAndExplicitWrites(t *testing.T) {
	s, _ := newWorkspaceTestStore(t)
	a := saveTestScript(t, s, "Clientes", "one")
	for _, title := range []string{"", strings.Repeat("á", 121), " clientes "} {
		if _, err := s.SaveScript(SavedScript{Title: title, Kind: Postgres, ProfileID: "one"}); err == nil {
			t.Fatalf("aceitou nome %q", title)
		}
	}
	saveTestScript(t, s, "Clientes", "two")
	before := a.UpdatedAt
	a.Text = "SELECT 2;"
	saved, err := s.SaveScript(a)
	if err != nil || !saved.UpdatedAt.After(before) {
		t.Fatal("não atualizou conteúdo e data")
	}
	renamed, err := s.RenameScript(a.ID, "  Relatório  ")
	if err != nil || renamed.Title != "Relatório" || renamed.Text != "SELECT 2;" {
		t.Fatal("renomear alterou conteúdo")
	}
	stale := WorkspaceState{Version: 2, OpenTabs: []OpenScript{{ID: 1, ScriptID: a.ID}}}
	if err := s.Save(stale); err != nil {
		t.Fatal(err)
	}
	loaded, _ := s.Load()
	if loaded.Workspace.Scripts[0].Title != "Relatório" || loaded.Workspace.Scripts[0].Text != "SELECT 2;" {
		t.Fatal("estado das abas sobrescreveu script")
	}
	if err := s.DeleteScript(a.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ = s.Load()
	if len(loaded.Workspace.Scripts) != 1 || len(loaded.Workspace.OpenTabs) != 0 {
		t.Fatal("exclusão não removeu referências")
	}
	if err := s.Save(stale); err == nil {
		t.Fatal("aceitou referência excluída")
	}
	if _, err := s.SaveScript(a); err == nil {
		t.Fatal("recriou script excluído por atualização atrasada")
	}
}
func TestWorkspaceMigratesLegacyWithoutLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	content := `{"version":1,"activeTabID":7,"scripts":[{"id":3,"title":"Clientes","text":"SELECT 'ação';","kind":"postgresql","profileID":"saved","head":9},{"id":7,"title":"clientes","text":"SELECT 2;","kind":"postgresql","profileID":"saved"},{"id":8,"title":"","text":"{\"find\":\"clientes\"}","kind":"mongodb"}]}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewWorkspaceStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	w := loaded.Workspace
	if loaded.Warning == "" || w.Version != 2 || len(w.Scripts) != 3 || w.Scripts[0].Title != "Clientes" || w.Scripts[1].Title != "clientes (2)" || w.Scripts[2].Title != "Script importado" || w.ActiveTabID != 7 || w.OpenTabs[0].Head != 9 {
		t.Fatalf("migração: %+v", w)
	}
	backups, _ := filepath.Glob(path + ".v1-backup-*")
	if len(backups) != 1 {
		t.Fatal("cópia ausente")
	}
	b, _ := os.ReadFile(backups[0])
	if string(b) != content {
		t.Fatal("cópia alterada")
	}
	reloaded, err := NewWorkspaceStore(path).Load()
	if err != nil || reloaded.Warning != "" || reloaded.Workspace.Scripts[0].ID != w.Scripts[0].ID {
		t.Fatal("migração repetida")
	}
}
func TestInvalidWorkspacePreservesOriginal(t *testing.T) {
	for _, content := range []string{"invalid", `{"version":99}`, `{"version":1,"scripts":[{"id":1,"kind":"invalid"}]}`, `{"version":2,"openTabs":[{"id":1,"scriptID":"missing"}]}`} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "workspace.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := NewWorkspaceStore(path).Load()
			if err != nil || loaded.Warning == "" || len(loaded.Workspace.Scripts) != 0 {
				t.Fatal("não recuperou arquivo inválido")
			}
			backups, _ := filepath.Glob(path + ".invalid-*")
			if len(backups) != 1 {
				t.Fatal("cópia ausente")
			}
			b, _ := os.ReadFile(backups[0])
			if string(b) != content {
				t.Fatal("cópia alterada")
			}
		})
	}
}
func TestWorkspaceRejectsFailedWritesWithoutChangingMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	s := NewWorkspaceStore(path)
	if _, err := s.SaveScript(SavedScript{Title: "A", Kind: Postgres}); err == nil {
		t.Fatal("salvou antes de carregar")
	}
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	a := saveTestScript(t, s, "A", "")
	before, _ := os.ReadFile(path)
	if err := s.Save(WorkspaceState{Version: 2, OpenTabs: []OpenScript{{ID: 1, ScriptID: a.ID}, {ID: 1, ScriptID: a.ID}}}); err == nil {
		t.Fatal("aceitou abas duplicadas")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("alterou arquivo inválido")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	a.Text = "não deve gravar"
	if _, err := s.SaveScript(a); err == nil {
		t.Fatal("ignorou falha de disco")
	}
	loaded, _ := s.Load()
	if loaded.Workspace.Scripts[0].Text == a.Text {
		t.Fatal("memória avançou após falha")
	}
}
func TestConcurrentTabAndScriptWritesPreserveContent(t *testing.T) {
	s, path := newWorkspaceTestStore(t)
	a := saveTestScript(t, s, "A", "")
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			if err := s.Save(WorkspaceState{Version: 2, OpenTabs: []OpenScript{{ID: 1, ScriptID: a.ID, Head: i}}}); err != nil {
				t.Error(err)
			}
		})
	}
	a.Text = "última versão explícita"
	if _, err := s.SaveScript(a); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	loaded, err := NewWorkspaceStore(path).Load()
	if err != nil || loaded.Workspace.Scripts[0].Text != a.Text {
		t.Fatal("perdeu conteúdo")
	}
	b, _ := json.Marshal(loaded.Workspace)
	var independent Workspace
	_ = json.Unmarshal(b, &independent)
	independent.Scripts[0].Text = "externo"
	loaded.Workspace.Scripts[0].Text = "externo"
	current, _ := s.Load()
	if current.Workspace.Scripts[0].Text != a.Text {
		t.Fatal("Load expôs estado interno")
	}
}
