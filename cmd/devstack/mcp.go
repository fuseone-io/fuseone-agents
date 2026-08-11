package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serveMCP runs a small CRM over stdio.
//
// One tool writes. A stack where every tool is a read never reaches an
// approval, so the platform's central behaviour — a run stopping to ask a
// person, and that person seeing what it will do — could not be exercised
// locally at all. It arrives unclassified like any imported tool and only
// becomes a write once the Curator says so, which is the point.
func serveMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "crm", Version: "0.1.0-dev"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "lookup",
		Description: "Find a customer account by email address",
	}, lookup)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the knowledge base for articles matching a query",
	}, search)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reply",
		Description: "Send a reply to the customer on the support ticket",
	}, sendReply)

	return server.Run(context.Background(), &mcp.StdioTransport{})
}

type lookupIn struct {
	Email string `json:"email" jsonschema:"the customer's email address"`
}

type lookupOut struct {
	Account string `json:"account"`
	Name    string `json:"name"`
	Plan    string `json:"plan"`
	Status  string `json:"status"`
}

func lookup(_ context.Context, _ *mcp.CallToolRequest, in lookupIn) (*mcp.CallToolResult, lookupOut, error) {
	if in.Email == "" {
		return nil, lookupOut{}, fmt.Errorf("email is required")
	}
	return nil, lookupOut{
		Account: "acct_4471", Name: "Marina Reis",
		Plan: "enterprise", Status: "active",
	}, nil
}

type searchIn struct {
	Query string `json:"query" jsonschema:"what to search the knowledge base for"`
}

type searchOut struct {
	Articles []string `json:"articles"`
}

func search(_ context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
	return nil, searchOut{Articles: []string{
		"KB-118: emitir segunda via de boleto",
		"KB-204: prazos de compensação",
	}}, nil
}

type sendReplyIn struct {
	Account string `json:"account" jsonschema:"the account the reply belongs to"`
	Subject string `json:"subject" jsonschema:"the subject line"`
	Body    string `json:"body" jsonschema:"what to say to the customer"`
}

type sendReplyOut struct {
	MessageID string `json:"message_id"`
	SentTo    string `json:"sent_to"`
}

func sendReply(_ context.Context, _ *mcp.CallToolRequest, in sendReplyIn) (*mcp.CallToolResult, sendReplyOut, error) {
	if in.Body == "" {
		return nil, sendReplyOut{}, fmt.Errorf("body is required")
	}
	return nil, sendReplyOut{MessageID: "msg_9f21", SentTo: in.Account}, nil
}
