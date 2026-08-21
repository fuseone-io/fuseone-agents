package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/connect"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/settings"
)

// refusalAlertSweep is slower than approvals and faster than "somebody will
// notice later". It reads from a tiny projection after import, not from a fold
// of the ledger.
const refusalAlertSweep = 30 * time.Second

const (
	refusalAlertLease   = time.Minute
	refusalAlertHorizon = 5 * time.Second
)

func watchPolicyRefusals(
	ctx context.Context, store *settings.Store, pool *pgxpool.Pool, baseURL, owner string,
) {
	forms := policy.NewRefusalForms(pool)
	notice := channel.NewNotice(
		channel.NewConfigured(store),
		channel.NewRouter(connect.New(store)),
	)

	sweep(ctx, refusalAlertSweep, "new Gate refusals announced", func() (int, error) {
		until := time.Now().Add(-refusalAlertHorizon)
		if _, err := forms.Import(ctx, until, 200); err != nil {
			return 0, err
		}
		pending, err := forms.Claim(ctx, owner, refusalAlertLease, 20)
		if err != nil {
			return 0, err
		}

		sent := 0
		failures := []error{}
		for _, form := range pending {
			n, err := notice.AnnounceCount(ctx, form.Scope, refusalAlertMessage(form, baseURL))
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if err := forms.MarkAnnounced(ctx, form, owner, time.Now()); err != nil {
				if errors.Is(err, policy.ErrRefusalClaimLost) {
					failures = append(failures, err)
					continue
				}
				return sent, err
			}
			sent += n
		}
		return sent, errors.Join(failures...)
	})
}

func refusalAlertMessage(form policy.RefusalForm, baseURL string) channel.Message {
	reason := form.PolicyCode
	if reason == "" {
		reason = form.Rule
	}
	if form.Effect.String() != "" {
		reason = fmt.Sprintf("%s blocked %s", reasonOrUnknown(reason), form.Effect)
	}
	m := channel.Message{
		Event:  channel.EventGateRefusal,
		RunID:  form.FirstRunID,
		Agent:  form.FirstAgentID,
		Scope:  form.Scope,
		Tool:   string(form.Tool),
		Reason: reason,
	}
	if baseURL != "" {
		m.Link = fmt.Sprintf("%s/runs/%s", baseURL, form.FirstRunID)
	}
	return m
}

func reasonOrUnknown(reason string) string {
	if reason == "" {
		return "Gate"
	}
	return reason
}
