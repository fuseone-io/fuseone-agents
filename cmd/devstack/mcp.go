package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serveMCP runs a small CRM over stdio.
//
// Every tool here is a read. Writes are deliberately absent: a tool imported
// from a server is read-only until the Curator promotes it, that promotion has
// no persistent home yet, and a development stack that quietly granted write
// access would teach the wrong thing about how the platform behaves.
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
