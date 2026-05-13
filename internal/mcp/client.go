package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Client struct {
	endpoint  string
	apiKey    string
	transport string
	http      *http.Client
	nextID    atomic.Int64
	sessionID string
	connected bool
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

type ToolResult struct {
	Content string
	IsError bool
}

func NewClient(endpoint, apiKey, transport string) *Client {
	return &Client{
		endpoint:  strings.TrimSuffix(endpoint, "/"),
		apiKey:    apiKey,
		transport: transport,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Connected() bool { return c.connected }

func (c *Client) Connect(ctx context.Context) error {
	req := c.newRequest("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "rig",
			"version": "0.1.0",
		},
	})

	_, err := c.send(ctx, req)
	if err != nil {
		c.connected = false
		return fmt.Errorf("mcp initialize: %w", err)
	}

	// Send initialized notification
	notif := c.newNotification("notifications/initialized")
	_ = c.sendNotification(ctx, notif)

	c.connected = true
	return nil
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	req := c.newRequest("tools/list", nil)
	resp, err := c.send(ctx, req)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing tools/list: %w", err)
	}
	return result.Tools, nil
}

func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	req := c.newRequest("resources/list", nil)
	resp, err := c.send(ctx, req)
	if err != nil {
		return nil, err
	}

	var result struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing resources/list: %w", err)
	}
	return result.Resources, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	params := map[string]any{
		"name": name,
	}
	if args != nil {
		params["arguments"] = args
	}

	req := c.newRequest("tools/call", params)
	resp, err := c.send(ctx, req)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, err
	}

	var texts []string
	for _, c := range result.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	return ToolResult{
		Content: strings.Join(texts, "\n"),
		IsError: result.IsError,
	}, nil
}

func (c *Client) ReadResource(ctx context.Context, uri string) (string, error) {
	req := c.newRequest("resources/read", map[string]any{
		"uri": uri,
	})
	resp, err := c.send(ctx, req)
	if err != nil {
		return "", err
	}

	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing resources/read: %w", err)
	}
	if len(result.Contents) == 0 {
		return "", nil
	}
	return result.Contents[0].Text, nil
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) newRequest(method string, params any) jsonRPCRequest {
	return jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  method,
		Params:  params,
	}
}

func (c *Client) newNotification(method string) jsonRPCRequest {
	return jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
	}
}

func (c *Client) send(ctx context.Context, rpcReq jsonRPCRequest) (json.RawMessage, error) {
	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		return c.parseSSEResponse(resp.Body, rpcReq.ID)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing json-rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func (c *Client) parseSSEResponse(r io.Reader, requestID int64) (json.RawMessage, error) {
	scanner := bufio.NewScanner(r)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if line == "" && len(dataLines) > 0 {
			// End of an SSE event — try to parse the accumulated data
			data := strings.Join(dataLines, "\n")
			dataLines = nil

			var rpcResp jsonRPCResponse
			if err := json.Unmarshal([]byte(data), &rpcResp); err != nil {
				continue
			}
			// Match the response to our request ID
			if rpcResp.ID == requestID {
				if rpcResp.Error != nil {
					return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
				}
				return rpcResp.Result, nil
			}
		}
	}

	// Handle any trailing data without a final blank line
	if len(dataLines) > 0 {
		data := strings.Join(dataLines, "\n")
		var rpcResp jsonRPCResponse
		if err := json.Unmarshal([]byte(data), &rpcResp); err != nil {
			return nil, fmt.Errorf("parsing sse json-rpc: %w", err)
		}
		if rpcResp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
		}
		return rpcResp.Result, nil
	}

	return nil, fmt.Errorf("no matching response in sse stream for request %d", requestID)
}

func (c *Client) sendNotification(ctx context.Context, notif jsonRPCRequest) error {
	body, err := json.Marshal(notif)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}
	resp.Body.Close()
	return nil
}
