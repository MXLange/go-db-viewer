package drivers

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBridgeHelper(t *testing.T) {
	if os.Getenv("DBVIEWER_TEST_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req bridgeRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		switch req.Action {
		case "hang":
			time.Sleep(time.Hour)
		case "failure":
			_, _ = os.Stdout.WriteString("{\"ok\":false}\n")
		default:
			_, _ = os.Stdout.WriteString("{\"ok\":true,\"result\":{\"message\":\"ok\"}}\n")
		}
	}
	os.Exit(0)
}

func testBridge(t *testing.T) *bridgeSession {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBridgeHelper$")
	cmd.Env = append(os.Environ(), "DBVIEWER_TEST_HELPER=1")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &bridgeSession{cmd: cmd, in: in, scanner: bufio.NewScanner(out), done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(s.done) }()
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBridgeProtocolAndSafeErrors(t *testing.T) {
	s := testBridge(t)
	r, err := s.call(t.Context(), bridgeRequest{Action: "ping", DSN: "do-not-expose"})
	if err != nil || r.Result.Message != "ok" {
		t.Fatal("falha no protocolo")
	}
	_, err = s.call(t.Context(), bridgeRequest{Action: "failure", DSN: "do-not-expose"})
	if err == nil || strings.Contains(err.Error(), "do-not-expose") {
		t.Fatal("erro ausente ou contém a string")
	}
}

func TestBridgeTimeoutTerminatesProcess(t *testing.T) {
	s := testBridge(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := s.call(ctx, bridgeRequest{Action: "hang"})
	if err == nil || ctx.Err() == nil {
		t.Fatal("timeout não aplicado")
	}
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("processo não encerrado")
	}
	if _, err := s.call(t.Context(), bridgeRequest{Action: "ping"}); err == nil {
		t.Fatal("aceitou uma sessão encerrada")
	}
}
