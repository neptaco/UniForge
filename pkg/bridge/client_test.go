package bridge

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// newPipeClient builds a Client wired to an in-memory net.Pipe connection.
// Connect() is intentionally not used: these tests exercise request/Close only.
func newPipeClient(t *testing.T, requestTimeout time.Duration) (*Client, net.Conn) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	client := &Client{
		options: ClientOptions{RequestTimeout: requestTimeout},
		conn:    clientConn,
		reader:  bufio.NewReader(clientConn),
	}

	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})

	return client, serverConn
}

// startEchoServer serves requests one at a time, echoing the "marker" param
// back inside the result so a caller can verify it received its own response.
func startEchoServer(t *testing.T, serverConn net.Conn) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)

		scanner := bufio.NewScanner(serverConn)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var envelope rpcEnvelope
			if err := json.Unmarshal([]byte(line), &envelope); err != nil {
				return
			}

			var params struct {
				Marker int `json:"marker"`
			}
			if len(envelope.Params) > 0 {
				_ = json.Unmarshal(envelope.Params, &params)
			}

			if _, err := fmt.Fprintf(
				serverConn,
				`{"jsonrpc":"2.0","id":%s,"result":{"echo":%d}}`+"\n",
				string(envelope.ID), params.Marker,
			); err != nil {
				return
			}
		}
	}()

	t.Cleanup(func() {
		_ = serverConn.Close()
		<-done
	})
}

type echoResult struct {
	Echo int `json:"echo"`
}

func TestRequestDoesNotReuseConnectionAfterReadTimeout(t *testing.T) {
	client, serverConn := newPipeClient(t, 100*time.Millisecond)

	go func() {
		scanner := bufio.NewScanner(serverConn)
		if !scanner.Scan() {
			return
		}
		// Write a partial response (no trailing newline) and then stall.
		_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","id":"`))
	}()

	err := client.request("client.listProjects", nil, 100*time.Millisecond, &echoResult{})
	if err == nil {
		t.Fatal("expected the first request to fail with a read timeout")
	}
	if !strings.Contains(err.Error(), "failed to read daemon response") {
		t.Fatalf("expected a read failure, got: %v", err)
	}

	if client.conn != nil {
		t.Fatal("expected the connection to be dropped after a read failure")
	}
	if client.reader != nil {
		t.Fatal("expected the reader to be dropped after a read failure")
	}

	secondErr := client.request("client.listProjects", nil, 100*time.Millisecond, &echoResult{})
	if !errors.Is(secondErr, ErrConnectionInvalidated) {
		t.Fatalf("expected ErrConnectionInvalidated, got: %v", secondErr)
	}
	if strings.Contains(secondErr.Error(), "failed to decode") {
		t.Fatalf("expected no decode failure from leftover bytes, got: %v", secondErr)
	}
}

func TestRequestInvalidatesConnectionOnResponseIDMismatch(t *testing.T) {
	client, serverConn := newPipeClient(t, time.Second)

	go func() {
		scanner := bufio.NewScanner(serverConn)
		if !scanner.Scan() {
			return
		}
		_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","id":"someone-elses-id","result":{"echo":1}}` + "\n"))
	}()

	err := client.request("client.listProjects", nil, time.Second, &echoResult{})
	if err == nil {
		t.Fatal("expected an error for a mismatched response id")
	}
	if !strings.Contains(err.Error(), "unexpected daemon response id") {
		t.Fatalf("expected an id mismatch error, got: %v", err)
	}
	if client.conn != nil {
		t.Fatal("expected the connection to be dropped after an id mismatch")
	}

	secondErr := client.request("client.listProjects", nil, time.Second, &echoResult{})
	if !errors.Is(secondErr, ErrConnectionInvalidated) {
		t.Fatalf("expected ErrConnectionInvalidated, got: %v", secondErr)
	}
}

func TestRequestSkipsServerNotifications(t *testing.T) {
	client, serverConn := newPipeClient(t, time.Second)

	go func() {
		scanner := bufio.NewScanner(serverConn)
		envelope := rpcEnvelope{}
		if !scanner.Scan() {
			return
		}
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			return
		}

		if _, err := serverConn.Write([]byte(`{"jsonrpc":"2.0","method":"daemon.notify","params":{}}` + "\n")); err != nil {
			return
		}
		_, _ = fmt.Fprintf(serverConn, `{"jsonrpc":"2.0","id":%s,"result":{"echo":7}}`+"\n", string(envelope.ID))
	}()

	var result echoResult
	if err := client.request("client.listProjects", nil, time.Second, &result); err != nil {
		t.Fatalf("expected the request to succeed, got: %v", err)
	}
	if result.Echo != 7 {
		t.Fatalf("expected echo 7, got %d", result.Echo)
	}
}

func TestConcurrentRequestsDoNotCrossTalk(t *testing.T) {
	client, serverConn := newPipeClient(t, 5*time.Second)
	startEchoServer(t, serverConn)

	const goroutines = 20

	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	results := make([]echoResult, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			params := map[string]any{"marker": n}
			errs[n] = client.request("client.listProjects", params, 5*time.Second, &results[n])
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if results[i].Echo != i {
			t.Fatalf("goroutine %d: got echo %d, want %d", i, results[i].Echo, i)
		}
	}
}

func TestRequestAfterCloseReportsNotConnected(t *testing.T) {
	client, serverConn := newPipeClient(t, time.Second)
	_ = serverConn

	if err := client.Close(); err != nil {
		t.Fatalf("unexpected Close error: %v", err)
	}

	err := client.request("client.listProjects", nil, time.Second, &echoResult{})
	if err == nil {
		t.Fatal("expected an error after Close")
	}
	if !strings.Contains(err.Error(), "daemon client is not connected") {
		t.Fatalf("expected a not-connected error, got: %v", err)
	}
	if errors.Is(err, ErrConnectionInvalidated) {
		t.Fatalf("expected a plain not-connected error, got: %v", err)
	}
}

// A request must not be reported as connected once a previous request
// invalidated the connection, even if Close is called in between.
func TestCloseClearsInvalidatedState(t *testing.T) {
	client, serverConn := newPipeClient(t, 100*time.Millisecond)

	go func() {
		scanner := bufio.NewScanner(serverConn)
		if !scanner.Scan() {
			return
		}
		_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","id":"`))
	}()

	if err := client.request("client.listProjects", nil, 100*time.Millisecond, &echoResult{}); err == nil {
		t.Fatal("expected the first request to fail")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("unexpected Close error: %v", err)
	}

	err := client.request("client.listProjects", nil, 100*time.Millisecond, &echoResult{})
	if errors.Is(err, ErrConnectionInvalidated) {
		t.Fatalf("expected Close to clear the invalidated state, got: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "daemon client is not connected") {
		t.Fatalf("expected a not-connected error, got: %v", err)
	}
}
