package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"
)

type activeSession struct {
	mu      sync.Mutex
	session Session
	ctx     context.Context
	cancel  context.CancelFunc
}

type Service struct {
	settings *SettingsStore
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	store    *Store
	drivers  map[Kind]Driver
	sessions map[string]*activeSession
}

func NewService(ctx context.Context, store *Store, drivers ...Driver) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{ctx: ctx, cancel: cancel, store: store, drivers: map[Kind]Driver{}, sessions: map[string]*activeSession{}}
	for _, d := range drivers {
		s.drivers[d.Info().Kind] = d
	}
	return s
}

func NewServiceWithSettings(ctx context.Context, store *Store, settings *SettingsStore, drivers ...Driver) *Service {
	s := NewService(ctx, store, drivers...)
	s.settings = settings
	return s
}
func (s *Service) resolveRowLimit(requested int) (int, error) {
	fallback := DefaultRowLimit
	if requested == 0 && s.settings != nil {
		loaded, err := s.settings.Load()
		if err != nil {
			return 0, err
		}
		fallback = loaded.Settings.MaxRows
	}
	return ResolveRowLimit(requested, fallback)
}

func (s *Service) Drivers() []DriverInfo {
	var out []DriverInfo
	for _, kind := range []Kind{Postgres, Oracle, SQLServer, MongoDB, Ignite2, Ignite3} {
		if d := s.drivers[kind]; d != nil {
			out = append(out, d.Info())
		}
	}
	return out
}

func (s *Service) Profiles() ([]Profile, error)  { return s.store.List() }
func (s *Service) DeleteProfile(id string) error { return s.store.Delete(id) }

func (s *Service) resolve(input ConnectionInput) (ConnectionInput, Driver, error) {
	input, err := s.store.Resolve(input)
	if err != nil {
		return input, nil, err
	}
	d := s.drivers[input.Kind]
	if d == nil {
		return input, nil, errors.New("Tipo de banco inválido.")
	}
	if err := d.Validate(input.DSN); err != nil {
		return input, nil, err
	}
	return input, d, nil
}

func (s *Service) SaveProfile(input ConnectionInput) (Profile, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return Profile{}, errors.New("Informe o nome da conexão.")
	}
	resolved, _, err := s.resolve(input)
	if err != nil {
		return Profile{}, err
	}
	resolved.Name = input.Name
	return s.store.Save(resolved)
}

func (s *Service) Connect(input ConnectionInput) (string, error) {
	if s.ctx.Err() != nil {
		return "", errors.New("O aplicativo está encerrando.")
	}
	input, d, err := s.resolve(input)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	conn, err := d.Open(ctx, input.DSN)
	if err != nil {
		return "", safeConnectionError(err, "Falha ao conectar. Verifique endereço, credenciais, TLS e dependências do driver.")
	}
	if err = conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return "", safeConnectionError(err, "Falha no teste da conexão. Verifique endereço, porta, credenciais, TLS e disponibilidade do banco.")
	}
	base, stop := context.WithCancel(s.ctx)
	id := uuid.New().String()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		stop()
		_ = conn.Close()
		return "", errors.New("O aplicativo está encerrando.")
	}
	s.sessions[id] = &activeSession{session: conn, ctx: base, cancel: stop}
	return id, nil
}

func (s *Service) TestConnection(input ConnectionInput) error {
	id, err := s.Connect(input)
	if err != nil {
		return err
	}
	return s.Disconnect(id)
}

func (s *Service) Disconnect(id string) error {
	s.mu.Lock()
	active := s.sessions[id]
	delete(s.sessions, id)
	s.mu.Unlock()
	if active == nil {
		return nil
	}
	active.cancel()
	active.mu.Lock()
	defer active.mu.Unlock()
	return safeError(active.session.Close(), "Falha ao encerrar a conexão.")
}

func (s *Service) Close() {
	s.cancel()
	s.mu.Lock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.Disconnect(id)
	}
}

func (s *Service) acquire(id string) (*activeSession, context.Context, context.CancelFunc, error) {
	s.mu.Lock()
	active := s.sessions[id]
	s.mu.Unlock()
	if active == nil {
		return nil, nil, nil, errors.New("A conexão foi encerrada. Conecte novamente.")
	}
	active.mu.Lock()
	if active.ctx.Err() != nil {
		active.mu.Unlock()
		return nil, nil, nil, errors.New("A conexão foi encerrada.")
	}
	ctx, cancel := context.WithTimeout(active.ctx, 30*time.Second)
	return active, ctx, func() { cancel(); active.mu.Unlock() }, nil
}

func (s *Service) List(id string, parent Node) ([]Node, error) {
	a, ctx, done, err := s.acquire(id)
	if err != nil {
		return nil, err
	}
	defer done()
	nodes, err := a.session.List(ctx, parent)
	return nodes, safeError(err, "Não foi possível listar os objetos. Verifique as permissões.")
}

func (s *Service) Describe(id string, node Node) ([]Column, error) {
	a, ctx, done, err := s.acquire(id)
	if err != nil {
		return nil, err
	}
	defer done()
	columns, err := a.session.Describe(ctx, node)
	return columns, safeError(err, "Não foi possível descrever o objeto.")
}

func (s *Service) ListTables(id string) ([]TableSuggestion, error) {
	a, ctx, done, err := s.acquire(id)
	if err != nil {
		return nil, err
	}
	defer done()
	tables, err := a.session.ListTables(ctx)
	if err == nil {
		slices.SortFunc(tables, func(a, b TableSuggestion) int {
			return cmp.Or(strings.Compare(a.Schema, b.Schema), strings.Compare(a.Name, b.Name))
		})
	}
	return tables, safeError(err, "Não foi possível carregar as tabelas. Verifique a conexão e as permissões.")
}

func (s *Service) Preview(id string, req PreviewRequest) (Result, error) {
	limit, err := s.resolveRowLimit(req.RowLimit)
	if err != nil {
		return Result{}, err
	}
	req.RowLimit = limit
	if req.Page < 0 || req.Page > 100000 {
		return Result{}, errors.New("Página inválida.")
	}
	a, ctx, done, err := s.acquire(id)
	if err != nil {
		return Result{}, err
	}
	defer done()
	start := time.Now()
	r, err := a.session.Preview(ctx, req)
	r.ElapsedMS = time.Since(start).Milliseconds()
	return r, safeError(err, "Não foi possível visualizar os dados.")
}

func (s *Service) Execute(id string, req Request) (Result, error) {
	limit, err := s.resolveRowLimit(req.RowLimit)
	if err != nil {
		return Result{}, err
	}
	req.RowLimit = limit
	if strings.TrimSpace(req.Text) == "" {
		return Result{}, errors.New("Informe o comando.")
	}
	if len(req.Text) > 1<<20 {
		return Result{}, errors.New("O comando excede 1 MiB.")
	}
	a, ctx, done, err := s.acquire(id)
	if err != nil {
		return Result{}, err
	}
	defer done()
	start := time.Now()
	r, err := a.session.Execute(ctx, req)
	r.ElapsedMS = time.Since(start).Milliseconds()
	return r, safeError(err, "Falha na execução. Verifique a sintaxe e as permissões. O comando não será repetido.")
}

// SafeFault permite mensagens controladas, sem mensagens externas ou credenciais.
type SafeFault string

func (e SafeFault) Error() string { return string(e) }

func safeConnectionError(err error, fallback string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("Tempo limite da conexão excedido. Verifique se o banco está ativo e acessível no endereço e porta informados.")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("Conexão interrompida.")
	}
	return safeError(err, fallback)
}

func safeError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("Tempo limite de 30 segundos excedido. O estado de uma escrita pode ser incerto; confira os dados antes de repetir.")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("Operação interrompida. Confira os dados antes de repetir uma escrita.")
	}
	if fault, ok := errors.AsType[SafeFault](err); ok {
		return fault
	}
	type sqlState interface {
		error
		SQLState() string
	}
	if state, ok := errors.AsType[sqlState](err); ok {
		code := state.SQLState()
		if len(code) == 5 && strings.IndexFunc(code, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z') }) < 0 {
			return fmt.Errorf("%s SQLSTATE: %s.", fallback, code)
		}
	}
	return errors.New(fallback)
}

func (s *Service) ResolveTable(id string, ref TableReference) (Node, error) {
	if ref.Name == "" || len(ref.Name) > 1024 || len(ref.Schema) > 1024 || strings.ContainsRune(ref.Name+ref.Schema, 0) {
		return Node{}, SafeFault("Referência de tabela inválida.")
	}
	a, ctx, done, err := s.acquire(id)
	if err != nil {
		return Node{}, err
	}
	defer done()
	node, err := a.session.ResolveTable(ctx, ref)
	return node, safeError(err, "Não foi possível resolver a tabela. Verifique o schema e as permissões.")
}
func (s *Service) GetTableProperties(id string, node Node) (TableProperties, error) {
	a, ctx, done, err := s.acquire(id)
	if err != nil {
		return TableProperties{}, err
	}
	defer done()
	canonical, err := a.session.ResolveTable(ctx, TableReference{Schema: node.Schema, Name: node.Name, SchemaQuoted: true, NameQuoted: true})
	if err != nil {
		return TableProperties{}, safeError(err, "Tabela indisponível. Atualize os objetos.")
	}
	properties, err := a.session.GetTableProperties(ctx, canonical)
	return properties, safeError(err, "Não foi possível carregar as propriedades.")
}
