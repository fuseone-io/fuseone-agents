package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

/*
A refused cancellation is not a broken connection.

When a tool call outlives its deadline the SDK tells the server with a
"notifications/cancelled". Some servers answer that with 400 — Grafana's MCP
server does — and the SDK treats any refused message as fatal and closes the
session for good. Every later call then fails with "connection closed", so one
slow Loki query took a whole integration down.

Cancellation is advisory in MCP: the server may ignore it, and the call has
already been abandoned on this side. So a refusal of that one notification is
reported to the SDK as accepted. A 404 is left alone: it says the session is
gone, which is true and which the SDK knows how to handle.
*/
type tolerateRefusedCancel struct {
	server string
	base   http.RoundTripper
}

func withTolerantCancel(server string, client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{
			Transport: tolerateRefusedCancel{server: server, base: baseHTTPTransport()},
			Timeout:   60 * time.Second,
		}
	}
	out := *client
	base := out.Transport
	if base == nil {
		base = baseHTTPTransport()
	}
	out.Transport = tolerateRefusedCancel{server: server, base: base}
	return &out
}

func (t tolerateRefusedCancel) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodPost || r.Body == nil {
		return t.base.RoundTrip(r)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()

	out := r.Clone(r.Context())
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.ContentLength = int64(len(body))

	resp, err := t.base.RoundTrip(out)
	if err != nil || !refusedCancel(body, resp.StatusCode) {
		return resp, err
	}
	_ = resp.Body.Close()
	slog.Warn("tool server refused a call cancellation; the session is kept",
		"server", t.server, "status", resp.StatusCode)
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Status:     "202 Accepted",
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    r,
	}, nil
}

func refusedCancel(body []byte, status int) bool {
	if status < 400 || status == http.StatusNotFound {
		return false
	}
	var msg struct {
		Method string `json:"method"`
	}
	return json.Unmarshal(body, &msg) == nil && msg.Method == "notifications/cancelled"
}
