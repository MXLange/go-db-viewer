package core

import (
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"uuid"
)

const DefaultRowLimit = 200
const MaxRowLimit = 100000

type Settings struct {
	Theme   string `json:"theme"`
	MaxRows int    `json:"maxRows"`
}
type SettingsLoad struct {
	Settings Settings `json:"settings"`
	Warning  string   `json:"warning"`
}
type SettingsStore struct {
	mu      sync.Mutex
	path    string
	loaded  bool
	current SettingsLoad
}

func DefaultSettings() Settings                   { return Settings{Theme: "system", MaxRows: DefaultRowLimit} }
func NewSettingsStore(path string) *SettingsStore { return &SettingsStore{path: path} }
func ResolveRowLimit(requested, fallback int) (int, error) {
	if requested == 0 {
		requested = fallback
	}
	if requested < 1 || requested > MaxRowLimit {
		return 0, SafeFault("Informe um limite inteiro entre 1 e 100.000.")
	}
	return requested, nil
}
func validateSettings(settings Settings) error {
	if !slices.Contains([]string{"system", "light", "dark"}, settings.Theme) {
		return errors.New("Tema inválido. Escolha Sistema, Claro ou Escuro.")
	}
	if settings.MaxRows == 0 {
		return SafeFault("Informe um limite inteiro entre 1 e 100.000.")
	}
	_, err := ResolveRowLimit(settings.MaxRows, DefaultRowLimit)
	return err
}
func (s *SettingsStore) Load() (SettingsLoad, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.current, nil
	}
	loaded := SettingsLoad{Settings: DefaultSettings()}
	b, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return SettingsLoad{}, errors.New("Não foi possível ler as configurações. Verifique as permissões do diretório.")
	}
	if err == nil {
		var settings Settings
		if json.Unmarshal(b, &settings) != nil || validateSettings(settings) != nil {
			backup := s.path + ".invalid-" + uuid.New().String()
			if os.Rename(s.path, backup) != nil {
				return SettingsLoad{}, errors.New("Configurações inválidas. Não foi possível preservar uma cópia; o arquivo original foi mantido.")
			}
			loaded.Warning = "Configurações inválidas. Uma cópia foi preservada em " + filepath.Base(backup) + ". Os valores padrão foram carregados."
		} else {
			loaded.Settings = settings
		}
	}
	s.current = loaded
	s.loaded = true
	return loaded, nil
}
func (s *SettingsStore) Save(settings Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		return Settings{}, errors.New("Carregue as configurações anteriores antes de salvar.")
	}
	if err := validateSettings(settings); err != nil {
		return Settings{}, err
	}
	if err := s.write(settings); err != nil {
		return Settings{}, errors.New("Não foi possível salvar as configurações. Verifique espaço e permissões no diretório.")
	}
	s.current = SettingsLoad{Settings: settings}
	return settings, nil
}
func (s *SettingsStore) write(settings Settings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(settings, json.Deterministic(true))
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*")
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
