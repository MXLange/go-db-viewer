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

type Vault interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}

type Store struct {
	mu    sync.Mutex
	path  string
	vault Vault
}

func NewStore(path string, vault Vault) *Store { return &Store{path: path, vault: vault} }

func (s *Store) read() ([]Profile, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, errors.New("Não foi possível ler as conexões salvas.")
	}
	var profiles []Profile
	if json.Unmarshal(b, &profiles) != nil {
		return nil, errors.New("O arquivo de conexões está inválido.")
	}
	return profiles, nil
}

func (s *Store) write(profiles []Profile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(profiles, json.Deterministic(true))
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".connections-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}

func (s *Store) List() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Store) Save(input ConnectionInput) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.read()
	if err != nil {
		return Profile{}, err
	}
	id := input.ID
	if id == "" {
		id = uuid.New().String()
	}
	index := slices.IndexFunc(profiles, func(p Profile) bool { return p.ID == id })
	if input.ID != "" && index < 0 {
		return Profile{}, errors.New("Conexão salva não encontrada.")
	}
	dsn := input.DSN
	if dsn == "" {
		if index < 0 || profiles[index].Kind != input.Kind {
			return Profile{}, errors.New("Informe a string de conexão.")
		}
		dsn, err = s.vault.Get(id)
		if err != nil {
			return Profile{}, errors.New("Não foi possível acessar o cofre. Informe a string novamente.")
		}
	}
	old, oldErr := s.vault.Get(id)
	if index >= 0 && oldErr != nil {
		return Profile{}, errors.New("Não foi possível ler a credencial anterior. Verifique o cofre antes de alterar a conexão.")
	}
	if err = s.vault.Set(id, dsn); err != nil {
		return Profile{}, errors.New("O cofre está indisponível. Use Conectar sem salvar para esta sessão.")
	}
	p := Profile{ID: id, Name: input.Name, Kind: input.Kind, HasSecret: true}
	if index < 0 {
		profiles = append(profiles, p)
	} else {
		profiles[index] = p
	}
	if err = s.write(profiles); err != nil {
		if oldErr == nil {
			_ = s.vault.Set(id, old)
		} else {
			_ = s.vault.Delete(id)
		}
		return Profile{}, errors.New("Não foi possível gravar os metadados da conexão.")
	}
	return p, nil
}

func (s *Store) Resolve(input ConnectionInput) (ConnectionInput, error) {
	if input.DSN != "" {
		return input, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.read()
	if err != nil {
		return input, err
	}
	for _, p := range profiles {
		if p.ID == input.ID {
			if input.Kind != "" && input.Kind != p.Kind {
				return input, errors.New("Ao mudar o banco, informe uma nova string de conexão.")
			}
			input.Kind, input.Name = p.Kind, p.Name
			input.DSN, err = s.vault.Get(p.ID)
			if err != nil {
				return input, errors.New("Não foi possível acessar o cofre. Informe a string novamente.")
			}
			return input, nil
		}
	}
	return input, errors.New("Informe a string de conexão.")
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.read()
	if err != nil {
		return err
	}
	index := slices.IndexFunc(profiles, func(p Profile) bool { return p.ID == id })
	if index < 0 {
		return errors.New("Conexão salva não encontrada.")
	}
	old, err := s.vault.Get(id)
	if err != nil {
		return errors.New("Não foi possível acessar o cofre para remover a conexão.")
	}
	if err = s.vault.Delete(id); err != nil {
		return errors.New("Não foi possível remover a credencial do cofre.")
	}
	if err = s.write(slices.Delete(profiles, index, index+1)); err != nil {
		_ = s.vault.Set(id, old)
		return errors.New("Não foi possível remover os metadados da conexão.")
	}
	return nil
}
