package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type memoryVault struct {
	values   map[string]string
	fail     bool
	afterSet func()
}

func (v *memoryVault) Get(id string) (string, error) {
	value, ok := v.values[id]
	if !ok || v.fail {
		return "", errors.New("vault secret")
	}
	return value, nil
}
func (v *memoryVault) Set(id, secret string) error {
	if v.fail {
		return errors.New(secret)
	}
	v.values[id] = secret
	if v.afterSet != nil {
		v.afterSet()
	}
	return nil
}
func (v *memoryVault) Delete(id string) error {
	if v.fail {
		return errors.New("vault secret")
	}
	delete(v.values, id)
	return nil
}
func newTestStore(t *testing.T) (*Store, *memoryVault) {
	t.Helper()
	v := &memoryVault{values: map[string]string{}}
	return NewStore(filepath.Join(t.TempDir(), "connections.json"), v), v
}

func TestCredentialsStayInVault(t *testing.T) {
	s, v := newTestStore(t)
	p, err := s.Save(ConnectionInput{Name: "Teste", Kind: Postgres, DSN: "postgres://user:secret@host/db"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "postgres://") {
		t.Fatal("credencial gravada em arquivo")
	}
	if v.values[p.ID] != "postgres://user:secret@host/db" {
		t.Fatal("credencial ausente do cofre")
	}
	input, err := s.Resolve(ConnectionInput{ID: p.ID, Kind: Postgres})
	if err != nil || input.DSN == "" {
		t.Fatal("não resolveu a credencial")
	}
	if _, err := s.Resolve(ConnectionInput{ID: p.ID, Kind: MongoDB}); err == nil {
		t.Fatal("permitiu mudar o driver sem mudar a string")
	}
	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(v.values) != 0 {
		t.Fatal("credencial não removida")
	}
}

func TestVaultFailureDoesNotWritePlaintext(t *testing.T) {
	s, v := newTestStore(t)
	v.fail = true
	_, err := s.Save(ConnectionInput{Name: "Teste", Kind: Postgres, DSN: "top-secret"})
	if err == nil || strings.Contains(err.Error(), "top-secret") {
		t.Fatal("erro ausente ou contém a credencial")
	}
	if _, err := os.Stat(s.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("gravou arquivo sem cofre")
	}
}

func TestProfileRenameKeepsCredential(t *testing.T) {
	store, v := newTestStore(t)
	s := NewService(t.Context(), store, &fakeDriver{})
	p, err := s.SaveProfile(ConnectionInput{Name: "Original", Kind: Postgres, DSN: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.SaveProfile(ConnectionInput{ID: p.ID, Name: "Renomeada", Kind: Postgres})
	if err != nil || p.Name != "Renomeada" || v.values[p.ID] != "secret" {
		t.Fatal("alteração do nome não preservou a credencial")
	}
}

func TestMetadataFailureRestoresVault(t *testing.T) {
	s, v := newTestStore(t)
	p, err := s.Save(ConnectionInput{Name: "Teste", Kind: Postgres, DSN: "original"})
	if err != nil {
		t.Fatal(err)
	}
	v.afterSet = func() { _ = os.Remove(s.path); _ = os.Mkdir(s.path, 0700) }
	_, err = s.Save(ConnectionInput{ID: p.ID, Name: "Teste", Kind: Postgres, DSN: "changed"})
	if err == nil {
		t.Fatal("era esperado um erro na gravação")
	}
	if v.values[p.ID] != "original" {
		t.Fatal("credencial anterior não foi restaurada")
	}
}

func TestInvalidMetadataIsNotOverwritten(t *testing.T) {
	s, _ := newTestStore(t)
	if err := os.WriteFile(s.path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ConnectionInput{Name: "Teste", Kind: Postgres, DSN: "secret"}); err == nil {
		t.Fatal("sobrescreveu arquivo inválido")
	}
}

type fakeDriver struct {
	sessions []*fakeSession
	opens    int
}

func (d *fakeDriver) Info() DriverInfo      { return DriverInfo{Kind: Postgres} }
func (d *fakeDriver) Validate(string) error { return nil }
func (d *fakeDriver) Open(context.Context, string) (Session, error) {
	session := d.sessions[d.opens]
	d.opens++
	return session, nil
}

type fakeSession struct {
	pingErr         error
	metadataNode    Node
	metadataRef     TableReference
	propertiesCalls int
	calls           int
	closed          bool
	err             error
	entered         chan struct{}
	once            sync.Once
	label           string
}

func (*fakeSession) ListTables(context.Context) ([]TableSuggestion, error) {
	return []TableSuggestion{{Schema: "public", Name: "clientes", InsertText: `"public"."clientes"`}}, nil
}

func (s *fakeSession) Ping(context.Context) error                            { return s.pingErr }
func (*fakeSession) List(context.Context, Node) ([]Node, error)              { return nil, nil }
func (*fakeSession) Describe(context.Context, Node) ([]Column, error)        { return nil, nil }
func (*fakeSession) Preview(context.Context, PreviewRequest) (Result, error) { return Result{}, nil }
func (s *fakeSession) Execute(ctx context.Context, _ Request) (Result, error) {
	s.calls++
	if s.entered != nil {
		s.once.Do(func() { close(s.entered) })
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	return Result{Message: s.label}, s.err
}
func (s *fakeSession) Close() error { s.closed = true; return nil }

func TestExecutionFailureNeverRetriesAndHidesSecrets(t *testing.T) {
	store, _ := newTestStore(t)
	conn := &fakeSession{err: errors.New("postgres://user:secret@host/db")}
	driver := &fakeDriver{sessions: []*fakeSession{conn}}
	s := NewService(t.Context(), store, driver)
	defer s.Close()
	id, err := s.Connect(ConnectionInput{Kind: Postgres, DSN: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Execute(id, Request{Text: "DELETE FROM demo"})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("erro externo expôs dados da conexão")
	}
	if conn.calls != 1 {
		t.Fatalf("executou %d vezes", conn.calls)
	}
}

func TestSessionIsolationAndDisconnect(t *testing.T) {
	store, _ := newTestStore(t)
	a, b := &fakeSession{label: "A"}, &fakeSession{label: "B"}
	s := NewService(t.Context(), store, &fakeDriver{sessions: []*fakeSession{a, b}})
	defer s.Close()
	ida, _ := s.Connect(ConnectionInput{Kind: Postgres, DSN: "a"})
	idb, _ := s.Connect(ConnectionInput{Kind: Postgres, DSN: "b"})
	if ida == idb {
		t.Fatal("sessões com mesmo identificador")
	}
	if err := s.Disconnect(ida); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(ida, Request{Text: "SELECT 1"}); err == nil {
		t.Fatal("usou sessão encerrada")
	}
	r, err := s.Execute(idb, Request{Text: "SELECT 1"})
	if err != nil || r.Message != "B" || b.closed {
		t.Fatal("desconexão afetou outra sessão")
	}
}

func TestDisconnectCancelsActiveOperation(t *testing.T) {
	store, _ := newTestStore(t)
	conn := &fakeSession{entered: make(chan struct{})}
	s := NewService(t.Context(), store, &fakeDriver{sessions: []*fakeSession{conn}})
	id, err := s.Connect(ConnectionInput{Kind: Postgres, DSN: "test"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Execute(id, Request{Text: "SELECT 1"}); done <- err }()
	<-conn.entered
	if err := s.Disconnect(id); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil || !conn.closed {
		t.Fatal("não cancelou a operação e fechou a sessão")
	}
}

func TestTestConnectionClosesTemporarySession(t *testing.T) {
	store, _ := newTestStore(t)
	conn := &fakeSession{}
	s := NewService(t.Context(), store, &fakeDriver{sessions: []*fakeSession{conn}})
	if err := s.TestConnection(ConnectionInput{Kind: Postgres, DSN: "test"}); err != nil {
		t.Fatal(err)
	}
	if !conn.closed || len(s.sessions) != 0 {
		t.Fatal("teste deixou uma conexão aberta")
	}
}

func TestShutdownRejectsNewConnections(t *testing.T) {
	store, _ := newTestStore(t)
	driver := &fakeDriver{}
	s := NewService(t.Context(), store, driver)
	s.Close()
	if _, err := s.Connect(ConnectionInput{Kind: Postgres, DSN: "test"}); err == nil || driver.opens != 0 {
		t.Fatal("abriu uma conexão após encerrar o serviço")
	}
}

func TestCatalogRequiresActiveSession(t *testing.T) {
	store, _ := newTestStore(t)
	conn := &fakeSession{}
	s := NewService(t.Context(), store, &fakeDriver{sessions: []*fakeSession{conn}})
	defer s.Close()
	id, err := s.Connect(ConnectionInput{Kind: Postgres, DSN: "test"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.ListTables(id)
	if err != nil || len(items) != 1 || items[0].Name != "clientes" {
		t.Fatal("catálogo indisponível")
	}
	if err := s.Disconnect(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListTables(id); err == nil {
		t.Fatal("usou catálogo de sessão encerrada")
	}
}

func (s *fakeSession) ResolveTable(_ context.Context, ref TableReference) (Node, error) {
	s.metadataRef = ref
	return Node{Name: "table", Schema: "public", Kind: "table"}, s.err
}
func (s *fakeSession) GetTableProperties(_ context.Context, node Node) (TableProperties, error) {
	s.metadataNode = node
	s.propertiesCalls++
	return TableProperties{Table: node}, s.err
}

func TestTableMetadataRequiresSessionAndCanonicalObject(t *testing.T) {
	store, _ := newTestStore(t)
	conn := &fakeSession{}
	service := NewService(t.Context(), store, &fakeDriver{sessions: []*fakeSession{conn}})
	defer service.Close()
	id, err := service.Connect(ConnectionInput{Kind: Postgres, DSN: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ResolveTable(id, TableReference{}); err == nil {
		t.Fatal("aceitou nome vazio")
	}
	p, err := service.GetTableProperties(id, Node{Schema: "requested", Name: "object"})
	if err != nil || p.Table.Name != "table" || conn.metadataNode.Schema != "public" || !conn.metadataRef.NameQuoted || !conn.metadataRef.SchemaQuoted {
		t.Fatal("não resolveu o objeto canônico antes de ler metadados")
	}
	conn.err = errors.New("postgres://user:secret@host/db")
	if _, err = service.ResolveTable(id, TableReference{Name: "object"}); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("expôs erro externo")
	}
	if _, err = service.GetTableProperties(id, Node{Name: "object"}); err == nil || conn.propertiesCalls != 1 {
		t.Fatal("consultou propriedades após falha de resolução")
	}
	if err = service.Disconnect(id); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ResolveTable(id, TableReference{Name: "object"}); err == nil {
		t.Fatal("resolveu em sessão encerrada")
	}
	if _, err = service.GetTableProperties(id, Node{Name: "object"}); err == nil {
		t.Fatal("consultou sessão encerrada")
	}
}

func TestConnectionTimeoutDoesNotReportUncertainWrite(t *testing.T) {
	store, _ := newTestStore(t)
	conn := &fakeSession{pingErr: context.DeadlineExceeded}
	service := NewService(t.Context(), store, &fakeDriver{sessions: []*fakeSession{conn}})
	defer service.Close()
	_, err := service.Connect(ConnectionInput{Kind: Postgres, DSN: "test"})
	if err == nil || !strings.Contains(err.Error(), "conexão") || strings.Contains(err.Error(), "escrita") || !conn.closed || len(service.sessions) != 0 {
		t.Fatalf("timeout incorreto: %v", err)
	}
	// A execução mantém o aviso de escrita incerta.
	if err := safeError(context.DeadlineExceeded, "falha"); !strings.Contains(err.Error(), "escrita") {
		t.Fatal("removeu aviso de execução")
	}
	if err := safeConnectionError(context.Canceled, "falha"); strings.Contains(err.Error(), "escrita") {
		t.Fatal("cancelamento de conexão indica escrita")
	}
}
