package qoder

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateSessionUsesCNAgentReference(t *testing.T) {
	var got string
	client := NewClient("https://api.qoder.com.cn/api/v1/cloud", "pat", func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		got = string(body)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"sess"}`))}, nil
	})
	if _, err := client.CreateSession(context.Background(), "agent", "env"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"agent":{"id":"agent","type":"agent","version":1}`) {
		t.Fatalf("CN agent reference missing: %s", got)
	}
}
