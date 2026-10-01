package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

/*
A socket that stops speaking is a socket that stops delivering.

Slack does not retry Socket Mode events: whatever arrives while the connection
is half-open is simply lost, and a worker blocked in a read it will never be
answered is indistinguishable, from outside, from a quiet afternoon. So the
connection is given a deadline it has to be kept alive against, and Slack's own
pings are what keep it.
*/
func TestSlackSocket_aConnectionThatGoesSilent_isGivenUpAndOpenedAgain(t *testing.T) {
	t.Parallel()
	conns := make(chan *fakeSocket, 4)
	manager := testSocketManager(func() *fakeSocket {
		conn := newFakeSocket()
		conns <- conn
		return conn
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go manager.runOne(ctx, slackSocketTarget{name: "acme-slack", appToken: "xapp-1"})

	first := <-conns
	first.fail(errors.New("read tcp: i/o timeout"))
	select {
	case <-conns:
	case <-time.After(2 * time.Second):
		t.Fatal("the socket was not opened again after it went silent")
	}
	if first.deadlines() == 0 {
		t.Fatal("the connection was read with no deadline, so silence is never noticed")
	}
}

func TestSlackSocket_aPingFromSlack_answersAndPostponesTheDeadline(t *testing.T) {
	t.Parallel()
	conns := make(chan *fakeSocket, 4)
	manager := testSocketManager(func() *fakeSocket {
		conn := newFakeSocket()
		conns <- conn
		return conn
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go manager.runOne(ctx, slackSocketTarget{name: "acme-slack", appToken: "xapp-1"})

	conn := <-conns
	handler := conn.pingHandler()
	if handler == nil {
		t.Fatal("no ping handler, so Slack's keepalive does not keep anything alive")
	}
	before := conn.deadlines()
	if err := handler("keepalive"); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if conn.deadlines() <= before {
		t.Error("a ping did not postpone the read deadline")
	}
	if conn.pongs() != 1 {
		t.Error("a ping was not answered, so Slack closes the connection")
	}
}

func testSocketManager(build func() *fakeSocket) *slackSocketManager {
	return &slackSocketManager{
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		backoff: time.Millisecond,
		running: make(map[string]runningSlackSocket),
		openURL: func(context.Context, string) (string, error) { return "wss://slack.test/link", nil },
		dial: func(context.Context, string) (slackSocketConn, error) {
			return build(), nil
		},
	}
}

type fakeSocket struct {
	mu        sync.Mutex
	deadline  int
	pong      int
	onPing    func(string) error
	frames    chan []byte
	failure   chan error
	closeOnce sync.Once
}

func newFakeSocket() *fakeSocket {
	return &fakeSocket{frames: make(chan []byte), failure: make(chan error, 1)}
}

func (f *fakeSocket) ReadMessage() (int, []byte, error) {
	select {
	case body := <-f.frames:
		return websocket.TextMessage, body, nil
	case err := <-f.failure:
		return 0, nil, err
	}
}

func (f *fakeSocket) WriteMessage(int, []byte) error { return nil }
func (f *fakeSocket) SetReadLimit(int64)             {}

func (f *fakeSocket) SetReadDeadline(time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deadline++
	return nil
}

func (f *fakeSocket) SetPongHandler(func(string) error) {}

func (f *fakeSocket) SetPingHandler(handler func(string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onPing = handler
}

func (f *fakeSocket) WriteControl(messageType int, _ []byte, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if messageType == websocket.PongMessage {
		f.pong++
	}
	return nil
}

func (f *fakeSocket) Close() error {
	f.closeOnce.Do(func() { f.fail(errors.New("closed")) })
	return nil
}

func (f *fakeSocket) fail(err error) {
	select {
	case f.failure <- err:
	default:
	}
}

func (f *fakeSocket) deadlines() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deadline
}

func (f *fakeSocket) pongs() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pong
}

func (f *fakeSocket) pingHandler() func(string) error {
	for range 200 {
		f.mu.Lock()
		handler := f.onPing
		f.mu.Unlock()
		if handler != nil {
			return handler
		}
		time.Sleep(time.Millisecond)
	}
	return nil
}
