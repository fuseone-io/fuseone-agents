package connectortools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// cloudflareCommentPrefix marks an entry as automatic. The external expiry
// job prunes by this prefix and its timestamp; a hand-added entry without it
// is never touched, by this connector or by that job.
const cloudflareCommentPrefix = "fuseone:auto:"

const maxCloudflareReasonChars = 200

// CloudflareListClient is the one remote surface the blocker needs: read the
// configured list, append one entry. Declared here, by the consumer.
type CloudflareListClient interface {
	Items(ctx context.Context, cfg CloudflareConfig, credential SecretValue) ([]CloudflareListItem, error)
	AddItem(ctx context.Context, cfg CloudflareConfig, credential SecretValue, ip, comment string) (string, error)
	DeleteItem(ctx context.Context, cfg CloudflareConfig, credential SecretValue, id string) (string, error)
}

// CloudflareBlocker guards the block list. Every refusal lives here, in
// code, because the agent that asks for blocks is the component whose
// judgement this feature exists to bound.
type CloudflareBlocker struct {
	remote CloudflareListClient
	now    func() time.Time
}

func NewCloudflareBlocker(remote CloudflareListClient) *CloudflareBlocker {
	return &CloudflareBlocker{remote: remote, now: time.Now}
}

// withNow pins the clock for tests; the daily ceiling is a calendar question.
func (b *CloudflareBlocker) withNow(now func() time.Time) *CloudflareBlocker {
	b.now = now
	return b
}

// WithCloudflare enables the block-list native path.
func (l *Layer) WithCloudflare(blocker *CloudflareBlocker) *Layer {
	l.cloudflare = blocker
	return l
}

type cloudflareBlockArgs struct {
	IP     string `json:"ip"`
	Reason string `json:"reason,omitempty"`
}

func (l *Layer) invokeCloudflareNative(
	ctx context.Context, instance Instance, op connectors.Operation, call engine.Call,
) (engine.ToolResult, error) {
	if l.cloudflare == nil || l.cloudflare.remote == nil {
		return failed(CodeConnectorUnavailable), nil
	}
	credential := SecretValue{value: instance.Token}
	switch op.ID {
	case "cloudflare.list_blocks":
		if !decodeEmptyArgs(call.Args) {
			return failed(CodeConnectorBadArguments), nil
		}
		return l.cloudflareList(ctx, instance, credential, call)
	case "cloudflare.block_ip":
		args, ok := decodeCloudflareBlockArgs(call.Args)
		if !ok {
			return failed(CodeConnectorBadArguments), nil
		}
		return l.cloudflareBlock(ctx, instance, credential, call, args)
	case "cloudflare.unblock_ip":
		args, ok := decodeCloudflareBlockArgs(call.Args)
		if !ok {
			return failed(CodeConnectorBadArguments), nil
		}
		return l.cloudflareUnblock(ctx, instance, credential, call, args)
	default:
		return failed(CodeConnectorUnavailable), nil
	}
}

func (l *Layer) cloudflareList(
	ctx context.Context, instance Instance, credential SecretValue, call engine.Call,
) (engine.ToolResult, error) {
	items, err := l.cloudflare.remote.Items(ctx, instance.Cloudflare, credential)
	if err != nil {
		return cloudflareFailure(err)
	}
	entries := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entries = append(entries, map[string]any{
			"ip": item.IP, "comment": item.Comment, "createdOn": item.CreatedOn,
		})
	}
	// Comments are remote text that once came through a model: labeled
	// untrusted so checkTaint sees any write they later steer.
	return l.storeJSON(ctx, call, domain.NewLabels(domain.LabelUntrusted), map[string]any{
		"operation": "cloudflare.list_blocks",
		"entries":   entries,
	})
}

func (l *Layer) cloudflareBlock(
	ctx context.Context, instance Instance, credential SecretValue,
	call engine.Call, args cloudflareBlockArgs,
) (engine.ToolResult, error) {
	addr, err := netip.ParseAddr(args.IP)
	if err != nil {
		return failed(CodeConnectorBadArguments), nil
	}
	if refuseAlways(addr) || inProtectedRanges(addr, instance.Cloudflare.ProtectedRanges) {
		// The code says only that a guard refused. The model already holds
		// the address it asked for; repeating it in a log would be the only
		// copy outside the run's own record.
		return failed(CodeConnectorGuardRefused), nil
	}
	items, err := l.cloudflare.remote.Items(ctx, instance.Cloudflare, credential)
	if err != nil {
		return cloudflareFailure(err)
	}
	canonical := cloudflareEntryFor(addr)
	for _, item := range items {
		if cloudflareEntryCovers(item.IP, addr) {
			// The echoed comment is remote text: untrusted, like every
			// read this connector answers with stored content.
			return l.storeJSON(ctx, call, domain.NewLabels(domain.LabelUntrusted), map[string]any{
				"operation": "cloudflare.block_ip", "blocked": canonical,
				"alreadyBlocked": true, "comment": item.Comment,
			})
		}
	}
	today := l.cloudflare.now().UTC().Format("2006-01-02")
	if countAutomaticEntries(items, today) >= cloudflareDailyCap(instance.Cloudflare) {
		return failed(CodeConnectorGuardRefused), nil
	}
	comment := cloudflareComment(l.cloudflare.now().UTC(), args.Reason)
	operationID, err := l.cloudflare.remote.AddItem(ctx, instance.Cloudflare, credential, canonical, comment)
	if err != nil {
		return cloudflareFailure(err)
	}
	return l.storeJSON(ctx, call, domain.Labels{}, map[string]any{
		"operation": "cloudflare.block_ip", "blocked": canonical,
		"alreadyBlocked": false, "comment": comment, "operationId": operationID,
	})
}

/*
cloudflareUnblock removes one automatically blocked address.

The one guard that matters most: only an entry the connector itself wrote —
the fuseone:auto comment — may fall. A person's entry, or a person's wider
range that covers the address, is a person's decision; the agent reports it
and never undoes it. An address not on the list is already in the desired
state and succeeds saying so, so a retry never fails on its own success.
*/
func (l *Layer) cloudflareUnblock(
	ctx context.Context, instance Instance, credential SecretValue,
	call engine.Call, args cloudflareBlockArgs,
) (engine.ToolResult, error) {
	// Removing protection always carries a decision's identity. An allow
	// policy that lowers the Gate's ladder would otherwise let a misled run
	// strip the list unattended; requiring DecidedBy here is structural —
	// the call must have been released by a click or an explicit mandate,
	// and either one is named, counted and capped. The adversarial review
	// of this operation found the gap; this is its closure.
	if call.DecidedBy == "" {
		return failed(CodeConnectorNeedsDecision), nil
	}
	addr, err := netip.ParseAddr(args.IP)
	if err != nil {
		return failed(CodeConnectorBadArguments), nil
	}
	items, err := l.cloudflare.remote.Items(ctx, instance.Cloudflare, credential)
	if err != nil {
		return cloudflareFailure(err)
	}
	canonical := addr.Unmap().String()
	var exact *CloudflareListItem
	for i, item := range items {
		if existing, parseErr := netip.ParseAddr(item.IP); parseErr == nil &&
			existing.Unmap() == addr.Unmap() {
			exact = &items[i]
			continue
		}
		if prefix, parseErr := netip.ParsePrefix(item.IP); parseErr == nil &&
			prefix.Contains(addr.Unmap()) {
			// A range covering the address: ranges are never written by this
			// connector, so this is a person's decision and the address
			// stays blocked by it whatever happens to the exact entry.
			return failed(CodeConnectorGuardRefused), nil
		}
	}
	if exact == nil {
		return l.storeJSON(ctx, call, domain.Labels{}, map[string]any{
			"operation": "cloudflare.unblock_ip", "unblocked": canonical,
			"alreadyAbsent": true,
		})
	}
	if !strings.HasPrefix(exact.Comment, cloudflareCommentPrefix) {
		return failed(CodeConnectorGuardRefused), nil
	}
	operationID, err := l.cloudflare.remote.DeleteItem(ctx, instance.Cloudflare, credential, exact.ID)
	if err != nil {
		return cloudflareFailure(err)
	}
	// The removed entry's comment stays out of the result on purpose: it is
	// stored text that once came through a model, and echoing it back into
	// a model's context would hand stored content a second life as input.
	// The run's own ledger already holds why the block was made.
	return l.storeJSON(ctx, call, domain.Labels{}, map[string]any{
		"operation": "cloudflare.unblock_ip", "unblocked": canonical,
		"alreadyAbsent": false, "operationId": operationID,
	})
}

// refuseAlways is the set no configuration can unblock: an address in it is
// infrastructure or shared space, and blocking it is the self-inflicted
// outage this connector must make impossible, not merely unlikely.
func refuseAlways(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified()
}

func inProtectedRanges(addr netip.Addr, ranges []string) bool {
	addr = addr.Unmap()
	for _, raw := range ranges {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			// Validated at save time; an unparsable survivor fails closed.
			return true
		}
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func countAutomaticEntries(items []CloudflareListItem, day string) int {
	count := 0
	for _, item := range items {
		if strings.HasPrefix(item.Comment, cloudflareCommentPrefix+day) {
			count++
		}
	}
	return count
}

// cloudflareComment is `fuseone:auto:<RFC3339> <reason>`, one line of plain
// text. The reason is the model's words and is flattened accordingly; the
// timestamp is what the expiry job trusts.
func cloudflareComment(now time.Time, reason string) string {
	comment := cloudflareCommentPrefix + now.Format(time.RFC3339)
	reason = strings.Join(strings.Fields(reason), " ")
	if reason == "" {
		return comment
	}
	if len(reason) > maxCloudflareReasonChars {
		reason = reason[:maxCloudflareReasonChars]
	}
	return comment + " " + reason
}

func cloudflareFailure(err error) (engine.ToolResult, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return engine.ToolResult{}, err
	}
	// The class and never the body: an authorization refusal, a missing
	// list and a plain failure each send the operator to a different fix.
	var remote cloudflareRemoteError
	if errors.As(err, &remote) {
		switch remote.status {
		case 401, 403:
			return failed(CodeConnectorUpstreamAuth), nil
		case 404:
			return failed(CodeConnectorUpstreamNotFound), nil
		}
	}
	return failed(CodeConnectorUpstreamFailed), nil
}

// cloudflareEntryFor is what the list stores for one blocked actor: the
// address itself, IPv4 or IPv6 — verified against the real API, which
// accepts both as single entries.
func cloudflareEntryFor(addr netip.Addr) string {
	return addr.Unmap().String()
}

// cloudflareEntryCovers is the idempotence read: a stored entry covers the
// address when it is the same address or a prefix containing it — an
// operator may add a covering CIDR by hand, and a block inside it is
// already done.
func cloudflareEntryCovers(entry string, addr netip.Addr) bool {
	addr = addr.Unmap()
	if existing, err := netip.ParseAddr(entry); err == nil {
		return existing.Unmap() == addr
	}
	if prefix, err := netip.ParsePrefix(entry); err == nil {
		return prefix.Contains(addr)
	}
	return false
}

func decodeCloudflareBlockArgs(raw []byte) (cloudflareBlockArgs, bool) {
	var args cloudflareBlockArgs
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return cloudflareBlockArgs{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return cloudflareBlockArgs{}, false
	}
	if strings.TrimSpace(args.IP) == "" {
		return cloudflareBlockArgs{}, false
	}
	return args, true
}

func decodeEmptyArgs(raw []byte) bool {
	if len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&struct{}{}); err != nil {
		return false
	}
	return errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func cloudflareBlockSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ip": map[string]any{
				"type":        "string",
				"description": "One literal IPv4 or IPv6 address to block. Never a range, never a hostname.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": fmt.Sprintf("Short plain-text evidence for the block, at most %d characters.", maxCloudflareReasonChars),
			},
		},
		"required":             []string{"ip"},
		"additionalProperties": false,
	}
}

func cloudflareUnblockSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ip": map[string]any{
				"type":        "string",
				"description": "One literal IP address to remove from the block list. Only automatically blocked entries can be removed.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Short plain-text reason for restoring access.",
			},
		},
		"required":             []string{"ip"},
		"additionalProperties": false,
	}
}

func cloudflareListSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}
