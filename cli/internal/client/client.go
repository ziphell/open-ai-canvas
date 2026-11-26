package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Debug   bool
	Log     io.Writer
}

type Meta struct {
	RequestID  string `json:"requestId,omitempty"`
	DurationMS int64  `json:"durationMs,omitempty"`
}

type APIError struct {
	Status  int
	Code    int
	Reason  string
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return e.Reason
	}
	return e.Message
}

type envelope struct {
	Code   int             `json:"code"`
	Data   json.RawMessage `json:"data"`
	Msg    string          `json:"msg"`
	Reason string          `json:"reason"`
}

func New(baseURL, token string, timeout time.Duration, insecure, debug bool, log io.Writer) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- explicit local debugging only.
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: timeout, Transport: transport},
		Debug:   debug,
		Log:     log,
	}
}

func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, headers map[string]string) (json.RawMessage, Meta, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, Meta{}, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := c.newRequest(ctx, method, path, query, reader)
	if err != nil {
		return nil, Meta{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, Meta{}, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, Meta{}, err
	}
	meta := Meta{DurationMS: time.Since(start).Milliseconds(), RequestID: resp.Header.Get("X-Correlation-ID")}
	if c.Debug {
		fmt.Fprintf(c.Log, "> %s %s\n< %d %s\n", method, req.URL.Path, resp.StatusCode, meta.RequestID)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, meta, decodeError(resp.StatusCode, payload)
	}
	if len(payload) == 0 {
		return nil, meta, nil
	}
	var env envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, meta, fmt.Errorf("响应不是有效 JSON（HTTP %d）", resp.StatusCode)
	}
	if env.Code != 0 {
		return nil, meta, &APIError{Status: resp.StatusCode, Code: env.Code, Reason: env.Reason, Message: env.Msg}
	}
	return env.Data, meta, nil
}

func (c *Client) DoRaw(ctx context.Context, path string, query url.Values) ([]byte, string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", decodeError(resp.StatusCode, payload)
	}
	return payload, resp.Header.Get("Content-Type"), nil
}

func (c *Client) Upload(ctx context.Context, filePath, kind string, fields map[string]string, headers map[string]string) (json.RawMessage, Meta, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, Meta{}, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, Meta{}, err
		}
	}
	if kind != "" {
		if err := writer.WriteField("kind", kind); err != nil {
			return nil, Meta{}, err
		}
	}
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, Meta{}, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, Meta{}, err
	}
	if err := writer.Close(); err != nil {
		return nil, Meta{}, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/resources", nil, &body)
	if err != nil {
		return nil, Meta{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, Meta{}, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, Meta{}, err
	}
	meta := Meta{DurationMS: time.Since(start).Milliseconds(), RequestID: resp.Header.Get("X-Correlation-ID")}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, meta, decodeError(resp.StatusCode, payload)
	}
	var env envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, meta, fmt.Errorf("上传响应不是有效 JSON（HTTP %d）", resp.StatusCode)
	}
	if env.Code != 0 {
		return nil, meta, &APIError{Status: resp.StatusCode, Code: env.Code, Reason: env.Reason, Message: env.Msg}
	}
	return env.Data, meta, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	if c.BaseURL == "" {
		return nil, errors.New("未配置 base URL")
	}
	endpoint := c.BaseURL + "/api" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "yingce-cli/0.1")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return req, nil
}

func decodeError(status int, payload []byte) error {
	var env envelope
	if len(payload) > 0 && json.Unmarshal(payload, &env) == nil && (env.Msg != "" || env.Reason != "") {
		return &APIError{Status: status, Code: env.Code, Reason: env.Reason, Message: env.Msg}
	}
	message := strings.TrimSpace(string(payload))
	if len(message) > 300 {
		message = message[:300]
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &APIError{Status: status, Message: message}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.Reason == "quota_exceeded" || apiErr.Code == 40301 {
			return 8
		}
		switch apiErr.Status {
		case http.StatusBadRequest:
			return 2
		case http.StatusUnauthorized:
			return 3
		case http.StatusForbidden:
			return 4
		case http.StatusNotFound:
			return 5
		case http.StatusConflict:
			return 6
		case http.StatusTooManyRequests:
			return 7
		}
		if apiErr.Code >= 500 || apiErr.Status >= 500 {
			return 9
		}
		return 9
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return 10
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 10
	}
	return 9
}
