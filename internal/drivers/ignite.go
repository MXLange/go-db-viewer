package drivers

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"go-db-viewer/internal/core"
)

type IgniteDriver struct{ Kind core.Kind }

func (d IgniteDriver) Info() core.DriverInfo {
	info := (SQLDriver{Kind: d.Kind}).Info()
	if runtime.GOOS == "darwin" && !odbcBuilt {
		info.Example = "jdbc:ignite:thin://localhost:10800/PUBLIC?user=usuario&password=senha"
		if d.Kind == core.Ignite3 {
			info.Example = "jdbc:ignite:thin://localhost:10800?username=usuario&password=senha"
		}
	}
	return info
}
func (d IgniteDriver) Validate(dsn string) error {
	if strings.HasPrefix(dsn, "jdbc:ignite:thin://") {
		if runtime.GOOS != "darwin" {
			return core.SafeFault("Use uma string ODBC neste sistema.")
		}
		if len(dsn) > 65536 {
			return core.SafeFault("A string de conexão excede 64 KiB.")
		}
		return nil
	}
	return (SQLDriver{Kind: d.Kind}).Validate(dsn)
}
func (d IgniteDriver) Open(ctx context.Context, dsn string) (core.Session, error) {
	if strings.HasPrefix(dsn, "jdbc:") {
		return startBridge(ctx, d.Kind, dsn)
	}
	if !odbcAvailable(dsn) {
		return nil, core.SafeFault("ODBC indisponível neste build. Instale unixODBC e compile com -tags odbc. No macOS, use JDBC com Java e o processo auxiliar.")
	}
	session, err := (SQLDriver{Kind: d.Kind, DriverName: "odbc"}).Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	// Não trocar de transporte após erro de autenticação, rede ou execução.
	return session, nil
}

type bridgeRequest struct {
	Reference core.TableReference `json:"reference"`
	Action    string              `json:"action"`
	Kind      core.Kind           `json:"kind,omitempty"`
	DSN       string              `json:"dsn,omitempty"`
	Node      core.Node           `json:"node"`
	Preview   core.PreviewRequest `json:"preview"`
	Request   core.Request        `json:"request"`
}
type bridgeResponse struct {
	Node       core.Node              `json:"node"`
	Properties core.TableProperties   `json:"properties"`
	OK         bool                   `json:"ok"`
	Result     core.Result            `json:"result"`
	Nodes      []core.Node            `json:"nodes"`
	Columns    []core.Column          `json:"columns"`
	Tables     []core.TableSuggestion `json:"tables"`
}
type bridgeSession struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	in      io.WriteCloser
	scanner *bufio.Scanner
	done    chan struct{}
	stop    sync.Once
}

func startBridge(ctx context.Context, kind core.Kind, dsn string) (core.Session, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	jar := os.Getenv("GO_DB_VIEWER_IGNITE_BRIDGE")
	if jar == "" {
		jar = filepath.Join(filepath.Dir(path), "ignite-bridge.jar")
		if _, err := os.Stat(jar); err != nil {
			if runtime.GOOS == "darwin" {
				jar = filepath.Join(filepath.Dir(path), "..", "Resources", "ignite-bridge.jar")
			}
		}
	}
	if _, err := os.Stat(jar); err != nil {
		return nil, core.SafeFault("Processo auxiliar ausente. Compile ignite-bridge e configure GO_DB_VIEWER_IGNITE_BRIDGE ou instale o JAR junto ao aplicativo.")
	}
	cmd := exec.Command("java", "--add-opens=java.base/java.nio=ALL-UNNAMED", "--add-opens=java.base/sun.nio.ch=ALL-UNNAMED", "-jar", jar)
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		return nil, core.SafeFault("Java não está disponível. Instale Java 17 ou superior para usar Ignite via JDBC.")
	}
	s := &bridgeSession{cmd: cmd, in: in, scanner: bufio.NewScanner(out), done: make(chan struct{})}
	s.scanner.Buffer(make([]byte, 65536), 32<<20)
	go func() { _ = cmd.Wait(); close(s.done) }()
	if _, err = s.call(ctx, bridgeRequest{Action: "open", Kind: kind, DSN: dsn}); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}
func (s *bridgeSession) call(ctx context.Context, req bridgeRequest) (bridgeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return bridgeResponse{}, err
	}
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()
	b, err := json.Marshal(req)
	if err != nil {
		return bridgeResponse{}, err
	}
	if _, err = s.in.Write(append(b, '\n')); err != nil {
		return bridgeResponse{}, err
	}
	if !s.scanner.Scan() {
		if ctx.Err() != nil {
			return bridgeResponse{}, ctx.Err()
		}
		return bridgeResponse{}, core.SafeFault("O processo Java encerrou. Confira os dados antes de repetir uma escrita e reconecte.")
	}
	var response bridgeResponse
	if json.Unmarshal(s.scanner.Bytes(), &response) != nil {
		return response, errors.New("Resposta inválida do processo Java.")
	}
	if !response.OK {
		return response, errors.New("Falha JDBC.")
	}
	return response, nil
}
func (s *bridgeSession) Ping(ctx context.Context) error {
	_, err := s.call(ctx, bridgeRequest{Action: "ping"})
	return err
}
func (s *bridgeSession) Close() error {
	s.stop.Do(func() { _ = s.in.Close(); _ = s.cmd.Process.Kill() })
	<-s.done
	return nil
}
func (s *bridgeSession) List(ctx context.Context, n core.Node) ([]core.Node, error) {
	r, err := s.call(ctx, bridgeRequest{Action: "list", Node: n})
	return r.Nodes, err
}
func (s *bridgeSession) Describe(ctx context.Context, n core.Node) ([]core.Column, error) {
	r, err := s.call(ctx, bridgeRequest{Action: "describe", Node: n})
	return r.Columns, err
}

func (s *bridgeSession) ListTables(ctx context.Context) ([]core.TableSuggestion, error) {
	r, err := s.call(ctx, bridgeRequest{Action: "tables"})
	return r.Tables, err
}
func (s *bridgeSession) Preview(ctx context.Context, r core.PreviewRequest) (core.Result, error) {
	limit, err := core.ResolveRowLimit(r.RowLimit, core.DefaultRowLimit)
	if err != nil {
		return core.Result{}, err
	}
	r.RowLimit = limit
	result, err := s.call(ctx, bridgeRequest{Action: "preview", Preview: r})
	return result.Result, err
}
func (s *bridgeSession) Execute(ctx context.Context, r core.Request) (core.Result, error) {
	limit, err := core.ResolveRowLimit(r.RowLimit, core.DefaultRowLimit)
	if err != nil {
		return core.Result{}, err
	}
	r.RowLimit = limit
	if err := singleStatement(r.Text); err != nil {
		return core.Result{}, err
	}
	result, err := s.call(ctx, bridgeRequest{Action: "execute", Request: r})
	return result.Result, err
}

func (s *bridgeSession) ResolveTable(ctx context.Context, ref core.TableReference) (core.Node, error) {
	r, err := s.call(ctx, bridgeRequest{Action: "resolve", Reference: ref})
	return r.Node, err
}
func (s *bridgeSession) GetTableProperties(ctx context.Context, node core.Node) (core.TableProperties, error) {
	r, err := s.call(ctx, bridgeRequest{Action: "properties", Node: node})
	return r.Properties, err
}
