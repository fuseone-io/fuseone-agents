package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

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
	// Over HTTP when an address is given, so the development stack can
	// exercise the remote transport as well as the local one. A stack that
	// could only speak stdio would leave the path most installations will
	// actually use tested nowhere but in production.
	addr := fs.String("addr", "", "serve over HTTP at this address instead of stdio")
	token := fs.String("token", "", "require this bearer token, when serving over HTTP")
	// Which set of tools this process offers.
	//
	// Two servers rather than one with everything, because a tool is named
	// after the server it came from: `crm.reply` and `erp.transfer` read as
	// what they are, and `lab.transfer` reads as a fixture. Policies are
	// written against `crm.*`, so the shape of the identifier is part of what
	// the lab is demonstrating.
	profile := fs.String("profile", "crm", "which tools to offer: crm or erp")
	if err := fs.Parse(args); err != nil {
		return err
	}

	server, err := serverFor(*profile)
	if err != nil {
		return err
	}

	if *addr == "" {
		return server.Run(context.Background(), &mcp.StdioTransport{})
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil)
	fmt.Fprintf(os.Stderr, "devstack mcp (%s) listening on %s\n", *profile, *addr)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           requireToken(*token, handler),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

// serverFor builds one of the two stand-ins.
func serverFor(profile string) (*mcp.Server, error) {
	switch profile {
	case "crm":
		return crmServer(), nil
	case "erp":
		server := mcp.NewServer(&mcp.Implementation{Name: "erp", Version: "0.1.0-dev"}, nil)
		newERP().register(server)
		return server, nil
	default:
		return nil, fmt.Errorf("unknown profile %q: crm or erp", profile)
	}
}

func crmServer() *mcp.Server {
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

	// The undo for reply, so the development stack has a real compensating
	// pair. Without one, abandoning a run locally can only ever report that
	// nothing takes anything back, and the half of SE-08 that matters — the
	// undo actually reaching a server — is unreachable outside the tests.
	//
	// It takes what reply returned. That convention is what lets the Curator
	// declare a compensator without anybody writing a mapping between two
	// schemas, and the stack should demonstrate the shape it expects.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "retract",
		Description: "Retract a reply already sent to the customer",
	}, retractReply)

	return server
}

// requireToken refuses a request without the bearer the operator configured.
//
// Present so the remote path is exercised with a credential rather than
// without one: a transport that silently dropped the token would pass every
// test written against a server that does not ask for it.
func requireToken(want string, next http.Handler) http.Handler {
	if want == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+want {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
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

// retractIn is sendReplyOut: the undo is called with what the do returned.
type retractIn struct {
	MessageID string `json:"message_id" jsonschema:"the reply to retract"`
	SentTo    string `json:"sent_to" jsonschema:"who it went to"`
}

type retractOut struct {
	Retracted bool `json:"retracted"`
}

func retractReply(_ context.Context, _ *mcp.CallToolRequest, in retractIn) (*mcp.CallToolResult, retractOut, error) {
	if in.MessageID == "" {
		return nil, retractOut{}, fmt.Errorf("message_id is required")
	}
	fmt.Fprintf(os.Stderr, "devstack: retracted %s\n", in.MessageID)
	return nil, retractOut{Retracted: true}, nil
}
