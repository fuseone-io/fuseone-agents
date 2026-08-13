package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

/*
An ERP, for the verdict the CRM cannot reach.

The support tools exercise three of the Gate's four answers: a read is allowed,
a write asks, and a proposal the pack does not cover is refused. The fourth —
an effect blocked outright by the risk ladder — needs a tool that moves money,
and nothing in a customer-support server plausibly does.

It also carries real state. `retract` in the CRM reports success and changes
nothing, which is enough to prove an undo reaches a server and not enough to
prove it undid anything. A transfer that a balance can be read before and after
is what makes a compensation demonstrable rather than asserted.
*/

type erp struct {
	mu        sync.Mutex
	balances  map[string]int64
	transfers map[string]transfer
	next      int
}

type transfer struct {
	From, To string
	Cents    int64
	Reversed bool
}

func newERP() *erp {
	return &erp{
		// Two accounts, funded. A lab where the first transfer fails for lack
		// of funds teaches somebody about this fixture rather than about the
		// platform.
		balances:  map[string]int64{"acct_4471": 500_000, "acct_9002": 120_000},
		transfers: map[string]transfer{},
	}
}

func (e *erp) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "balance",
		Description: "Read an account's balance, in cents",
	}, e.balance)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "transfer",
		Description: "Move money between two accounts",
	}, e.transfer)

	// Takes what transfer returned, which is the convention that lets a
	// Curator declare a compensator without anybody writing a mapping between
	// two schemas.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "refund",
		Description: "Reverse a transfer already made",
	}, e.refund)
}

type balanceIn struct {
	Account string `json:"account" jsonschema:"the account to read"`
}

type balanceOut struct {
	Account string `json:"account"`
	Cents   int64  `json:"cents"`
}

func (e *erp) balance(_ context.Context, _ *mcp.CallToolRequest, in balanceIn) (*mcp.CallToolResult, balanceOut, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cents, known := e.balances[in.Account]
	if !known {
		return nil, balanceOut{}, fmt.Errorf("no such account: %s", in.Account)
	}
	return nil, balanceOut{Account: in.Account, Cents: cents}, nil
}

type transferIn struct {
	From   string `json:"from" jsonschema:"the account to debit"`
	To     string `json:"to" jsonschema:"the account to credit"`
	Cents  int64  `json:"cents" jsonschema:"how much to move, in cents"`
	Reason string `json:"reason,omitempty" jsonschema:"why"`
}

// transferOut is refundIn: the undo is called with what the do returned.
type transferOut struct {
	TransferID string `json:"transfer_id"`
}

func (e *erp) transfer(_ context.Context, _ *mcp.CallToolRequest, in transferIn) (*mcp.CallToolResult, transferOut, error) {
	if in.Cents <= 0 {
		return nil, transferOut{}, fmt.Errorf("cents must be positive")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, known := e.balances[in.From]; !known {
		return nil, transferOut{}, fmt.Errorf("no such account: %s", in.From)
	}
	if _, known := e.balances[in.To]; !known {
		return nil, transferOut{}, fmt.Errorf("no such account: %s", in.To)
	}
	if e.balances[in.From] < in.Cents {
		return nil, transferOut{}, fmt.Errorf("insufficient funds in %s", in.From)
	}

	e.next++
	id := fmt.Sprintf("trf_%04d", e.next)
	e.balances[in.From] -= in.Cents
	e.balances[in.To] += in.Cents
	e.transfers[id] = transfer{From: in.From, To: in.To, Cents: in.Cents}

	return nil, transferOut{TransferID: id}, nil
}

type refundIn struct {
	TransferID string `json:"transfer_id" jsonschema:"the transfer to reverse"`
}

type refundOut struct {
	Reversed bool  `json:"reversed"`
	Cents    int64 `json:"cents"`
}

func (e *erp) refund(_ context.Context, _ *mcp.CallToolRequest, in refundIn) (*mcp.CallToolResult, refundOut, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	t, known := e.transfers[in.TransferID]
	if !known {
		return nil, refundOut{}, fmt.Errorf("no such transfer: %s", in.TransferID)
	}
	// Reversing twice must not move the money twice. The compensation sweep
	// can run again after a crash, and a fixture that double-refunded would
	// teach the wrong lesson about idempotence.
	if t.Reversed {
		return nil, refundOut{Reversed: true, Cents: t.Cents}, nil
	}

	e.balances[t.From] += t.Cents
	e.balances[t.To] -= t.Cents
	t.Reversed = true
	e.transfers[in.TransferID] = t

	return nil, refundOut{Reversed: true, Cents: t.Cents}, nil
}
