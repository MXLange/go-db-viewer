package core

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"uuid"
)

// ScriptTab representa o formato legado, usado somente na migração.
type ScriptTab struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	Text           string `json:"text"`
	Kind           Kind   `json:"kind"`
	ProfileID      string `json:"profileID"`
	ConnectionName string `json:"connectionName"`
	Database       string `json:"database"`
	Anchor         int    `json:"anchor"`
	Head           int    `json:"head"`
}

// ScriptInput contém somente os campos editáveis do script.
type ScriptInput struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Text           string `json:"text"`
	Kind           Kind   `json:"kind"`
	ProfileID      string `json:"profileID"`
	ConnectionName string `json:"connectionName"`
	Database       string `json:"database"`
}
type SavedScript struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Text           string    `json:"text"`
	Kind           Kind      `json:"kind"`
	ProfileID      string    `json:"profileID"`
	ConnectionName string    `json:"connectionName"`
	Database       string    `json:"database"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
type OpenScript struct {
	ID       int    `json:"id"`
	ScriptID string `json:"scriptID"`
	Anchor   int    `json:"anchor"`
	Head     int    `json:"head"`
}
type WorkspaceState struct {
	Version     int          `json:"version"`
	ActiveTabID int          `json:"activeTabID"`
	OpenTabs    []OpenScript `json:"openTabs"`
}
type Workspace struct {
	Version     int           `json:"version"`
	ActiveTabID int           `json:"activeTabID"`
	Scripts     []SavedScript `json:"scripts"`
	OpenTabs    []OpenScript  `json:"openTabs"`
}
type WorkspaceLoad struct {
	Workspace Workspace `json:"workspace"`
	Warning   string    `json:"warning"`
}
type WorkspaceStore struct {
	mu     sync.Mutex
	path   string
	loaded bool
	state  Workspace
}

func NewWorkspaceStore(path string) *WorkspaceStore { return &WorkspaceStore{path: path} }
func emptyWorkspace() Workspace {
	return Workspace{Version: 2, Scripts: []SavedScript{}, OpenTabs: []OpenScript{}}
}
func validKind(kind Kind) bool {
	return slices.Contains([]Kind{Postgres, Oracle, SQLServer, MongoDB, Ignite2, Ignite3}, kind)
}
func cloneWorkspace(w Workspace) Workspace {
	w.Scripts = slices.Clone(w.Scripts)
	w.OpenTabs = slices.Clone(w.OpenTabs)
	return w
}
func validateTitle(w Workspace, script SavedScript) error {
	if script.Title != strings.TrimSpace(script.Title) || !utf8.ValidString(script.Title) || utf8.RuneCountInString(script.Title) < 1 || utf8.RuneCountInString(script.Title) > 120 {
		return errors.New("Informe um nome de script com 1 a 120 caracteres.")
	}
	for _, other := range w.Scripts {
		if other.ID != script.ID && other.ProfileID == script.ProfileID && strings.EqualFold(other.Title, script.Title) {
			return errors.New("Já existe um script com este nome nesta conexão.")
		}
	}
	return nil
}
func validateWorkspace(w *Workspace) error {
	if w.Scripts == nil {
		w.Scripts = []SavedScript{}
	}
	if w.OpenTabs == nil {
		w.OpenTabs = []OpenScript{}
	}
	if w.Version != 2 {
		return errors.New("Versão do workspace inválida.")
	}
	scripts := map[string]bool{}
	for _, script := range w.Scripts {
		if script.ID == "" || scripts[script.ID] || !validKind(script.Kind) {
			return errors.New("Dados da biblioteca inválidos.")
		}
		if err := validateTitle(*w, script); err != nil {
			return err
		}
		scripts[script.ID] = true
	}
	ids := map[int]bool{}
	opened := map[string]bool{}
	for _, tab := range w.OpenTabs {
		if tab.ID <= 0 || ids[tab.ID] || !scripts[tab.ScriptID] || opened[tab.ScriptID] || tab.Anchor < 0 || tab.Head < 0 {
			return errors.New("Dados das abas inválidos.")
		}
		ids[tab.ID] = true
		opened[tab.ScriptID] = true
	}
	if !ids[w.ActiveTabID] {
		w.ActiveTabID = 0
		if len(w.OpenTabs) > 0 {
			w.ActiveTabID = w.OpenTabs[0].ID
		}
	}
	return nil
}
func migrateWorkspace(b []byte) (Workspace, error) {
	var old struct {
		Version     int         `json:"version"`
		ActiveTabID int         `json:"activeTabID"`
		Scripts     []ScriptTab `json:"scripts"`
	}
	if err := json.Unmarshal(b, &old); err != nil {
		return Workspace{}, err
	}
	w := emptyWorkspace()
	w.ActiveTabID = old.ActiveTabID
	ids := map[int]bool{}
	for _, tab := range old.Scripts {
		if tab.ID <= 0 || ids[tab.ID] || tab.Anchor < 0 || tab.Head < 0 || !validKind(tab.Kind) {
			return Workspace{}, errors.New("Workspace legado inválido.")
		}
		ids[tab.ID] = true
		title := strings.TrimSpace(tab.Title)
		if title == "" {
			title = "Script importado"
		}
		title = string([]rune(title)[:min(120, utf8.RuneCountInString(title))])
		script := SavedScript{ID: uuid.New().String(), Title: title, Text: tab.Text, Kind: tab.Kind, ProfileID: tab.ProfileID, ConnectionName: tab.ConnectionName, Database: tab.Database, UpdatedAt: time.Now().UTC()}
		for suffix := 2; validateTitle(w, script) != nil; suffix++ {
			tail := fmt.Sprintf(" (%d)", suffix)
			script.Title = string([]rune(title)[:min(120-len(tail), utf8.RuneCountInString(title))]) + tail
		}
		w.Scripts = append(w.Scripts, script)
		w.OpenTabs = append(w.OpenTabs, OpenScript{ID: tab.ID, ScriptID: script.ID, Anchor: tab.Anchor, Head: tab.Head})
	}
	err := validateWorkspace(&w)
	return w, err
}
func (s *WorkspaceStore) Load() (WorkspaceLoad, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return WorkspaceLoad{Workspace: cloneWorkspace(s.state)}, nil
	}
	w := emptyWorkspace()
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.loaded = true
		s.state = w
		return WorkspaceLoad{Workspace: cloneWorkspace(w)}, nil
	}
	if err != nil {
		return WorkspaceLoad{}, errors.New("Não foi possível ler os scripts salvos. Verifique o diretório de configuração.")
	}
	var version struct {
		Version int `json:"version"`
	}
	decodeErr := json.Unmarshal(b, &version)
	warning := ""
	if decodeErr == nil && version.Version == 1 {
		w, decodeErr = migrateWorkspace(b)
		if decodeErr == nil {
			backup := s.path + ".v1-backup-" + uuid.New().String()
			if os.WriteFile(backup, b, 0600) != nil {
				return WorkspaceLoad{}, errors.New("Não foi possível preservar o workspace anterior. A migração não foi aplicada.")
			}
			if err := s.write(w); err != nil {
				return WorkspaceLoad{}, errors.New("Não foi possível migrar os scripts. O arquivo original foi mantido.")
			}
			warning = "Scripts anteriores importados para a biblioteca. Uma cópia do workspace original foi preservada."
		}
	} else if decodeErr == nil {
		decodeErr = json.Unmarshal(b, &w)
		if decodeErr == nil {
			decodeErr = validateWorkspace(&w)
		}
	}
	if decodeErr != nil {
		backup := s.path + ".invalid-" + uuid.New().String()
		if os.Rename(s.path, backup) != nil {
			return WorkspaceLoad{}, errors.New("O workspace está inválido e não foi possível preservar uma cópia. O arquivo original foi mantido.")
		}
		w = emptyWorkspace()
		warning = "O workspace estava inválido. Uma cópia foi preservada em " + filepath.Base(backup) + "."
	}
	s.loaded = true
	s.state = w
	return WorkspaceLoad{Workspace: cloneWorkspace(w), Warning: warning}, nil
}
func (s *WorkspaceStore) commit(w Workspace) error {
	if !s.loaded {
		return errors.New("Os scripts anteriores não foram carregados. Recarregue o workspace antes de salvar.")
	}
	if err := validateWorkspace(&w); err != nil {
		return err
	}
	if err := s.write(w); err != nil {
		return errors.New("Não foi possível salvar os scripts. Verifique espaço e permissões no diretório de configuração.")
	}
	s.state = cloneWorkspace(w)
	return nil
}

// Save altera somente as abas; nunca recebe o conteúdo da biblioteca.
func (s *WorkspaceStore) Save(state WorkspaceState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := cloneWorkspace(s.state)
	w.Version = state.Version
	w.ActiveTabID = state.ActiveTabID
	w.OpenTabs = slices.Clone(state.OpenTabs)
	return s.commit(w)
}
func (s *WorkspaceStore) SaveScript(script SavedScript) (SavedScript, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := cloneWorkspace(s.state)
	script.Title = strings.TrimSpace(script.Title)
	index := slices.IndexFunc(w.Scripts, func(other SavedScript) bool { return other.ID == script.ID })
	if script.ID != "" && index < 0 {
		return SavedScript{}, errors.New("Script salvo não encontrado.")
	}
	if index >= 0 && w.Scripts[index].Kind != script.Kind {
		return SavedScript{}, errors.New("O tipo do banco do script não pode ser alterado.")
	}
	if script.ID == "" {
		script.ID = uuid.New().String()
	}
	script.UpdatedAt = time.Now().UTC()
	if err := validateTitle(w, script); err != nil {
		return SavedScript{}, err
	}
	if index < 0 {
		w.Scripts = append(w.Scripts, script)
	} else {
		w.Scripts[index] = script
	}
	if err := s.commit(w); err != nil {
		return SavedScript{}, err
	}
	return script, nil
}
func (s *WorkspaceStore) RenameScript(id, title string) (SavedScript, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := cloneWorkspace(s.state)
	index := slices.IndexFunc(w.Scripts, func(other SavedScript) bool { return other.ID == id })
	if index < 0 {
		return SavedScript{}, errors.New("Script salvo não encontrado.")
	}
	script := w.Scripts[index]
	script.Title = strings.TrimSpace(title)
	script.UpdatedAt = time.Now().UTC()
	if err := validateTitle(w, script); err != nil {
		return SavedScript{}, err
	}
	w.Scripts[index] = script
	if err := s.commit(w); err != nil {
		return SavedScript{}, err
	}
	return script, nil
}
func (s *WorkspaceStore) DeleteScript(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := cloneWorkspace(s.state)
	if !slices.ContainsFunc(w.Scripts, func(script SavedScript) bool { return script.ID == id }) {
		return errors.New("Script salvo não encontrado.")
	}
	w.Scripts = slices.DeleteFunc(w.Scripts, func(script SavedScript) bool { return script.ID == id })
	w.OpenTabs = slices.DeleteFunc(w.OpenTabs, func(tab OpenScript) bool { return tab.ScriptID == id })
	return s.commit(w)
}
func (s *WorkspaceStore) write(w Workspace) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(w, json.Deterministic(true))
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".workspace-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
