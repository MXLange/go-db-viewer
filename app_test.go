package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-db-viewer/internal/core"
)

func TestCloseWaitsForWorkspaceSave(t *testing.T) {
	s := core.NewWorkspaceStore(filepath.Join(t.TempDir(), "workspace.json"))
	requests := 0
	quit := make(chan struct{})
	a := &App{workspace: s, quit: func() { close(quit) }, requestSave: func(context.Context) { requests++ }}
	if a.beforeClose(t.Context()) {
		t.Fatal("bloqueou o fechamento antes de iniciar a UI")
	}
	if _, err := a.LoadWorkspace(); err != nil {
		t.Fatal(err)
	}
	if !a.beforeClose(t.Context()) || requests != 1 {
		t.Fatal("não solicitou o salvamento final")
	}
	if err := a.SaveWorkspaceAndClose(core.WorkspaceState{Version: 99}); err == nil || a.allowClose.Load() {
		t.Fatal("permitiu fechar após falha de gravação")
	}
	script, err := a.SaveScript(core.ScriptInput{Title: "Última edição", Text: "SELECT 'fim';", Kind: core.Postgres})
	if err != nil {
		t.Fatal(err)
	}
	w := core.WorkspaceState{Version: 2, ActiveTabID: 1, OpenTabs: []core.OpenScript{{ID: 1, ScriptID: script.ID}}}
	if err := a.SaveWorkspaceAndClose(w); err != nil {
		t.Fatal(err)
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("não concluiu o fechamento")
	}
	if a.beforeClose(t.Context()) {
		t.Fatal("bloqueou o fechamento após salvar")
	}
	loaded, err := s.Load()
	if err != nil || loaded.Workspace.Scripts[0].Text != "SELECT 'fim';" {
		t.Fatal("fechou sem gravar a última edição")
	}
}

func TestSettingsBindingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	app := &App{settings: core.NewSettingsStore(path)}
	loaded, err := app.LoadSettings()
	if err != nil || loaded.Settings.MaxRows != 200 || loaded.Settings.Theme != "system" {
		t.Fatal("padrão incorreto")
	}
	saved, err := app.SaveSettings(core.Settings{Theme: "light", MaxRows: 1000})
	if err != nil || saved.MaxRows != 1000 {
		t.Fatal("não salvou configurações")
	}
	app.settings = core.NewSettingsStore(path)
	loaded, err = app.LoadSettings()
	if err != nil || loaded.Settings.Theme != "light" || loaded.Settings.MaxRows != 1000 {
		t.Fatal("não restaurou configurações")
	}
}
