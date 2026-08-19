package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// socketModeSweep is how often workers reconcile which Slack Socket Mode
// connections should be open. Short enough that switching a channel from HTTP
// to Socket Mode is live, long enough not to make the vault a metronome.
const socketModeSweep = 30 * time.Second

const socketReadLimit = 128 << 10

type slackSocketConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	SetReadLimit(limit int64)
	Close() error
}

type slackSocketTarget struct {
	name     string
	key      string
	appToken string
}

type slackSocketCandidate struct {
	name string
	key  string
}

type runningSlackSocket struct {
	key    string
	cancel context.CancelFunc
}

type slackSocketManager struct {
	settings *settings.Store
	inbox    *channel.Inbox
	seen     slack.SeenAccounts
	log      *slog.Logger

	openURL func(context.Context, string) (string, error)
	dial    func(context.Context, string) (slackSocketConn, error)

	running map[string]runningSlackSocket
}

func (p *workerParts) receiveSlackSockets(ctx context.Context) {
	manager := &slackSocketManager{
		settings: p.settings,
		inbox:    channel.NewInbox(p.configPool),
		seen:     admin.NewChannels(p.configPool, p.settings),
		log:      slog.Default(),
		openURL: func(ctx context.Context, appToken string) (string, error) {
			return slack.OpenSocketURL(ctx, appToken, slack.SocketAPI,
				&http.Client{Timeout: 10 * time.Second})
		},
		dial: func(ctx context.Context, url string) (slackSocketConn, error) {
			conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
			return conn, err
		},
		running: make(map[string]runningSlackSocket),
	}
	manager.run(ctx)
}

func (m *slackSocketManager) run(ctx context.Context) {
	m.sync(ctx)

	ticker := time.NewTicker(socketModeSweep)
	defer ticker.Stop()
	defer m.stopAll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sync(ctx)
		}
	}
}

func (m *slackSocketManager) sync(ctx context.Context) {
	candidates, err := m.candidates(ctx)
	if err != nil {
		m.log.Error("slack socket configuration could not be read", "err", err)
		return
	}

	desired := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		desired[candidate.name] = struct{}{}
		if running, ok := m.running[candidate.name]; ok && running.key == candidate.key {
			continue
		}
		m.stop(candidate.name)
		target, ok := m.target(ctx, candidate)
		if !ok {
			continue
		}
		m.start(ctx, target)
	}
	for name := range m.running {
		if _, ok := desired[name]; !ok {
			m.stop(name)
		}
	}
}

func (m *slackSocketManager) candidates(ctx context.Context) ([]slackSocketCandidate, error) {
	stored, err := m.settings.List(ctx, channel.KindChannel)
	if err != nil {
		return nil, err
	}

	var out []slackSocketCandidate
	for _, set := range stored {
		if !set.Enabled {
			continue
		}
		var conn channel.Connection
		if err := json.Unmarshal(set.Value, &conn); err != nil {
			m.log.Warn("channel configuration is unreadable",
				"channel", set.Name, "err", err)
			continue
		}
		if conn.Kind != "slack" || channel.DeliveryMode(conn.DeliveryMode) != channel.DeliverySocket {
			continue
		}
		if !set.HasSecret {
			m.log.Warn("slack socket mode has no app-level token",
				"channel", set.Name)
			continue
		}
		out = append(out, slackSocketCandidate{
			name: set.Name,
			key:  set.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return out, nil
}

func (m *slackSocketManager) target(
	ctx context.Context, candidate slackSocketCandidate,
) (slackSocketTarget, bool) {
	held, err := m.settings.Reveal(ctx,
		settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, candidate.name)
	if err != nil {
		m.log.Error("slack socket token could not be read",
			"channel", candidate.name, "err", err)
		return slackSocketTarget{}, false
	}
	creds := channel.ReadCredentials(held.Secret)
	if strings.TrimSpace(creds.AppToken) == "" {
		m.log.Warn("slack socket mode has no app-level token",
			"channel", candidate.name)
		return slackSocketTarget{}, false
	}
	return slackSocketTarget{
		name: candidate.name, key: candidate.key, appToken: creds.AppToken,
	}, true
}

func (m *slackSocketManager) start(ctx context.Context, target slackSocketTarget) {
	child, cancel := context.WithCancel(ctx)
	m.running[target.name] = runningSlackSocket{key: target.key, cancel: cancel}
	go m.runOne(child, target)
}

func (m *slackSocketManager) stop(name string) {
	running, ok := m.running[name]
	if !ok {
		return
	}
	running.cancel()
	delete(m.running, name)
}

func (m *slackSocketManager) stopAll() {
	for name := range m.running {
		m.stop(name)
	}
}

func (m *slackSocketManager) runOne(ctx context.Context, target slackSocketTarget) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := m.connectOnce(ctx, target)
		if ctx.Err() != nil {
			return
		}
		m.log.Warn("slack socket disconnected",
			"channel", target.name, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > time.Minute {
			backoff = time.Minute
		}
	}
}

func (m *slackSocketManager) connectOnce(ctx context.Context, target slackSocketTarget) error {
	url, err := m.openURL(ctx, target.appToken)
	if err != nil {
		return err
	}
	conn, err := m.dial(ctx, url)
	if err != nil {
		return fmt.Errorf("slack: dial socket: %w", err)
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(socketReadLimit)

	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()

	receiver := slack.SocketReceiver{
		Channel: target.name, Inbox: m.inbox,
		Rules: channel.NewConfigured(m.settings), Seen: m.seen,
		Now: time.Now, Log: m.log,
	}
	for {
		_, body, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		ack, err := receiver.Handle(ctx, body)
		if err != nil {
			return err
		}
		if ack == nil {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, ack); err != nil {
			return fmt.Errorf("slack: acknowledge socket envelope: %w", err)
		}
	}
}
