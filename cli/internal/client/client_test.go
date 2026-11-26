package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoUnwrapsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"projects":[{"id":"p1"}]},"msg":"ok"}`))
	}))
	defer server.Close()

	api := New(server.URL, "test-token", 0, false, false, nil)
	data, _, err := api.Do(context.Background(), http.MethodGet, "/projects", nil, nil, nil)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if string(data) != `{"projects":[{"id":"p1"}]}` {
		t.Fatalf("unexpected data %s", data)
	}
}

func TestDoReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":40301,"msg":"积分不足","reason":"quota_exceeded"}`))
	}))
	defer server.Close()

	api := New(server.URL, "token", 0, false, false, nil)
	_, _, err := api.Do(context.Background(), http.MethodPost, "/tasks", nil, map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Reason != "quota_exceeded" {
		t.Fatalf("unexpected reason %s", apiErr.Reason)
	}
	if code := ExitCode(err); code != 8 {
		t.Fatalf("expected exit 8, got %d", code)
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		status int
		code   int
		want   int
	}{
		{http.StatusBadRequest, 400, 2},
		{http.StatusUnauthorized, 401, 3},
		{http.StatusForbidden, 403, 4},
		{http.StatusNotFound, 404, 5},
		{http.StatusConflict, 409, 6},
		{http.StatusTooManyRequests, 429, 7},
		{http.StatusInternalServerError, 500, 9},
	}
	for _, item := range cases {
		err := &APIError{Status: item.status, Code: item.code}
		if got := ExitCode(err); got != item.want {
			t.Fatalf("status %d: want %d, got %d", item.status, item.want, got)
		}
	}
}
