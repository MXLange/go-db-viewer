package main

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go-db-viewer/internal/core"
	"go-db-viewer/internal/drivers"
	"go-db-viewer/internal/vault"
)

type App struct {
	service         *core.Service
	cancel          context.CancelFunc
	workspace       *core.WorkspaceStore
	settings        *core.SettingsStore
	runtimeContext  context.Context
	workspaceActive atomic.Bool
	allowClose      atomic.Bool
	quit            func()
	requestSave     func(context.Context)
}

func NewApp() (*App, error) {
	path, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	store := core.NewStore(filepath.Join(path, "go-db-viewer", "connections.json"), vault.NativeVault{})
	settings := core.NewSettingsStore(filepath.Join(path, "go-db-viewer", "settings.json"))
	a := &App{settings: settings, service: core.NewServiceWithSettings(ctx, store, settings, drivers.Registry()...), cancel: cancel, workspace: core.NewWorkspaceStore(filepath.Join(path, "go-db-viewer", "workspace.json"))}
	a.quit = func() { runtime.Quit(a.runtimeContext) }
	a.requestSave = func(ctx context.Context) { runtime.EventsEmit(ctx, "workspace:save-before-close") }
	return a, nil
}
func (a *App) startup(ctx context.Context) { a.runtimeContext = ctx }
func (a *App) beforeClose(ctx context.Context) bool {
	if a.allowClose.Load() || !a.workspaceActive.Load() {
		return false
	}
	a.requestSave(ctx)
	return true
}
func (a *App) LoadWorkspace() (core.WorkspaceLoad, error) {
	a.workspaceActive.Store(true)
	return a.workspace.Load()
}
func (a *App) SaveWorkspace(w core.WorkspaceState) error { return a.workspace.Save(w) }
func (a *App) SaveWorkspaceAndClose(w core.WorkspaceState) error {
	if err := a.workspace.Save(w); err != nil {
		return err
	}
	a.allowClose.Store(true)
	go a.quit()
	return nil
}
func (a *App) ListTables(id string) ([]core.TableSuggestion, error) { return a.service.ListTables(id) }
func (a *App) shutdown(context.Context)                             { a.cancel(); a.service.Close() }
func (a *App) Drivers() []core.DriverInfo                           { return a.service.Drivers() }
func (a *App) Profiles() ([]core.Profile, error)                    { return a.service.Profiles() }
func (a *App) SaveProfile(input core.ConnectionInput) (core.Profile, error) {
	return a.service.SaveProfile(input)
}
func (a *App) DeleteProfile(id string) error { return a.service.DeleteProfile(id) }
func (a *App) TestConnection(input core.ConnectionInput) error {
	return a.service.TestConnection(input)
}
func (a *App) Connect(input core.ConnectionInput) (string, error) { return a.service.Connect(input) }
func (a *App) Disconnect(id string) error                         { return a.service.Disconnect(id) }
func (a *App) List(id string, parent core.Node) ([]core.Node, error) {
	return a.service.List(id, parent)
}
func (a *App) Describe(id string, node core.Node) ([]core.Column, error) {
	return a.service.Describe(id, node)
}
func (a *App) Preview(id string, request core.PreviewRequest) (core.Result, error) {
	return a.service.Preview(id, request)
}
func (a *App) Execute(id string, request core.Request) (core.Result, error) {
	return a.service.Execute(id, request)
}

func (a *App) ResolveTable(id string, ref core.TableReference) (core.Node, error) {
	return a.service.ResolveTable(id, ref)
}
func (a *App) GetTableProperties(id string, node core.Node) (core.TableProperties, error) {
	return a.service.GetTableProperties(id, node)
}

func (a *App) SaveScript(script core.ScriptInput) (core.SavedScript, error) {
	return a.workspace.SaveScript(core.SavedScript{ID: script.ID, Title: script.Title, Text: script.Text, Kind: script.Kind, ProfileID: script.ProfileID, ConnectionName: script.ConnectionName, Database: script.Database})
}
func (a *App) RenameScript(id, title string) (core.SavedScript, error) {
	return a.workspace.RenameScript(id, title)
}
func (a *App) DeleteScript(id string) error { return a.workspace.DeleteScript(id) }

func (a *App) LoadSettings() (core.SettingsLoad, error) { return a.settings.Load() }
func (a *App) SaveSettings(settings core.Settings) (core.Settings, error) {
	return a.settings.Save(settings)
}
