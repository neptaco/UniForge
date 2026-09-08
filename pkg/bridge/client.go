package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neptaco/uniforge/pkg/daemon"
)

const (
	defaultConnectTimeout = 2 * time.Second
	defaultRequestTimeout = 5 * time.Minute
)

// ErrConnectionInvalidated is returned when a previous request left the
// connection in an unknown state (write failure, read timeout, undecodable
// response, or a response id mismatch) and the connection was closed as a
// result. Call Connect again to establish a fresh connection before retrying.
var ErrConnectionInvalidated = errors.New("daemon connection was closed after a failed request; reconnect before retrying")

// ClientOptions configures a bridge Client.
type ClientOptions struct {
	DaemonConfig    daemon.Config
	AutoStartDaemon bool
	DaemonArgs      []string // args for starting daemon (e.g., ["daemon", "run"])
	ConnectTimeout  time.Duration
	RequestTimeout  time.Duration
}

type Client struct {
	options ClientOptions

	// mu serializes Connect/Close/request so that a single connection never has
	// more than one in-flight request and interleaved writes cannot happen.
	mu            sync.Mutex
	conn          net.Conn
	reader        *bufio.Reader
	invalidated   bool
	nextRequestID uint64
	clientID      string
}

func NewClient(options ClientOptions) *Client {
	if options.ConnectTimeout <= 0 {
		options.ConnectTimeout = defaultConnectTimeout
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = defaultRequestTimeout
	}
	if len(options.DaemonArgs) == 0 {
		options.DaemonArgs = []string{"daemon", "run"}
	}

	return &Client{
		options:  options,
		clientID: fmt.Sprintf("go-cli-%d", time.Now().UnixMilli()),
	}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	cfg := c.options.DaemonConfig

	conn, err := daemon.Dial(cfg, c.options.ConnectTimeout)
	if err != nil && c.options.AutoStartDaemon {
		if startErr := daemon.Start(context.Background(), cfg, daemon.StartOptions{
			Args: c.options.DaemonArgs,
		}); startErr != nil {
			return fmt.Errorf("failed to start daemon: %w", startErr)
		}
		conn, err = daemon.Dial(cfg, c.options.ConnectTimeout)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.invalidated = false
	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.invalidated = false

	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	c.reader = nil
	return err
}

// invalidateLocked closes and drops the current connection so that no
// subsequent request reads leftover bytes from a half-consumed response.
// The caller must hold c.mu.
func (c *Client) invalidateLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.reader = nil
	c.invalidated = true
}

func (c *Client) Register() (*ClientRegisterResult, error) {
	params := map[string]any{
		"clientId":        c.clientID,
		"protocolVersion": ProtocolVersion,
		"packageVersion":  PackageVersion,
		"buildTimestamp":  0,
	}

	var result ClientRegisterResult
	if err := c.request("client.register", params, c.options.RequestTimeout, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) ListProjects(includeTools bool) (*ClientListProjectsResult, error) {
	params := map[string]any{
		"includeTools": includeTools,
	}

	var result ClientListProjectsResult
	if err := c.request("client.listProjects", params, c.options.RequestTimeout, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) ToolCall(tool string, args map[string]any, projectID string, timeout time.Duration) (*ClientToolCallResult, error) {
	if timeout <= 0 {
		timeout = c.options.RequestTimeout
	}

	params := map[string]any{
		"tool": tool,
		"args": args,
	}
	if projectID != "" {
		params["projectId"] = projectID
	}
	if timeout > 0 {
		params["timeoutMs"] = int(timeout.Milliseconds())
	}

	var result ClientToolCallResult
	if err := c.request("client.toolCall", params, timeout, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) request(method string, params any, timeout time.Duration, target any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.reader == nil {
		if c.invalidated {
			return ErrConnectionInvalidated
		}
		return errors.New("daemon client is not connected")
	}

	requestID := fmt.Sprintf("go-%d-%d", time.Now().UnixMilli(), atomic.AddUint64(&c.nextRequestID, 1))
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := c.conn.Write(append(data, '\n')); err != nil {
		c.invalidateLocked()
		return fmt.Errorf("failed to send request: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		_ = c.conn.SetReadDeadline(deadline)
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			// The response line may have been partially consumed; the connection
			// can no longer be trusted for a subsequent request.
			c.invalidateLocked()
			if errors.Is(err, io.EOF) {
				return errors.New("daemon connection closed")
			}
			return fmt.Errorf("failed to read daemon response: %w", err)
		}

		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}

		var envelope rpcEnvelope
		if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
			c.invalidateLocked()
			return fmt.Errorf("failed to decode daemon response: %w", err)
		}

		if envelope.Method != "" {
			continue
		}

		if responseID := extractResponseID(envelope.ID); responseID != requestID {
			c.invalidateLocked()
			return fmt.Errorf("unexpected daemon response id %q (expected %q)", responseID, requestID)
		}

		if envelope.Error != nil {
			return &DaemonError{Code: envelope.Error.Code, Message: envelope.Error.Message}
		}

		if target == nil || len(envelope.Result) == 0 {
			return nil
		}

		if err := json.Unmarshal(envelope.Result, target); err != nil {
			return fmt.Errorf("failed to decode daemon result: %w", err)
		}

		return nil
	}
}
