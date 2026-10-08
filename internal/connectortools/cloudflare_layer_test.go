package connectortools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

type fakeCloudflareList struct {
	items     []CloudflareListItem
	itemsErr  error
	added     []CloudflareListItem
	deleted   []string
	deleteErr error
}

func (f *fakeCloudflareList) Items(
	_ context.Context, _ CloudflareConfig, _ SecretValue,
) ([]CloudflareListItem, error) {
	return f.items, f.itemsErr
}

func (f *fakeCloudflareList) AddItem(
	_ context.Context, _ CloudflareConfig, _ SecretValue, ip, comment string,
) (string, error) {
	f.added = append(f.added, CloudflareListItem{IP: ip, Comment: comment})
	return "op-1", nil
}

func cloudflareLayer(t *testing.T, remote *fakeCloudflareList) *Layer {
	t.Helper()
	layer := New(nil, nil, engine.NewMemoryContent(), nil).
		WithCloudflare(NewCloudflareBlocker(remote).withNow(func() time.Time {
			return time.Date(2026, 1, 10, 3, 0, 0, 0, time.UTC)
		}))
	instance := validCloudflareInstance()
	instance.Token = "CANARY-cf-token"
	layer.SetInstances([]Instance{instance})
	return layer
}

func blockCall(args string) engine.Call {
	return engine.Call{
		Tool: "cloudflare.edge.block_ip", RunID: "run-1", Seq: 7,
		Args: []byte(args),
	}
}

func resultBody(t *testing.T, layer *Layer, result engine.ToolResult) map[string]any {
	t.Helper()
	raw, err := layer.content.Get(t.Context(), result.ResultRef)
	if err != nil {
		t.Fatalf("content.Get: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return body
}

// The refusals that no configuration can switch off: an address that is
// private, loopback, link-local, multicast or unspecified is never blocked,
// whatever a run claims about it. Blocking an internal range through a
// misled model is the self-inflicted outage this guard exists to prevent.
func TestBlockIP_reservedAndPrivateAddresses_areRefusedInCode(t *testing.T) {
	t.Parallel()
	for name, ip := range map[string]string{
		"rfc1918 ten":         "10.1.2.3",
		"rfc1918 oneseventwo": "172.16.9.9",
		"rfc1918 oneninetwo":  "192.168.1.1",
		"loopback":            "127.0.0.1",
		"link local":          "169.254.169.254",
		"ipv6 loopback":       "::1",
		"ipv6 ula":            "fd00::1",
		"ipv6 link local":     "fe80::1",
		"multicast":           "224.0.0.1",
		"unspecified":         "0.0.0.0",
		"ipv4 mapped private": "::ffff:10.0.0.1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			remote := &fakeCloudflareList{}
			layer := cloudflareLayer(t, remote)
			result, err := layer.Invoke(t.Context(), blockCall(`{"ip":"`+ip+`"}`))
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
				t.Fatalf("result = %+v, want guard refusal", result)
			}
			if len(remote.added) != 0 {
				t.Fatal("the address reached the remote list")
			}
		})
	}
}

// The instance's own protected ranges — the installation's egress, an
// anonymizer whose addresses are shared by many real clients — refuse the
// same way.
func TestBlockIP_aProtectedRange_isRefused(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{}
	layer := cloudflareLayer(t, remote)
	// validCloudflareInstance protects 203.0.113.0/24 and 2001:db8::/32.
	for _, ip := range []string{"203.0.113.77", "2001:db8:1::9"} {
		result, err := layer.Invoke(t.Context(), blockCall(`{"ip":"`+ip+`"}`))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
			t.Fatalf("result = %+v, want guard refusal", result)
		}
	}
	if len(remote.added) != 0 {
		t.Fatal("a protected address reached the remote list")
	}
}

// Only a literal address is accepted: a CIDR, a hostname or junk is bad
// arguments, not a block of unknown width.
func TestBlockIP_anythingButALiteralAddress_isBadArguments(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{}
	layer := cloudflareLayer(t, remote)
	for name, args := range map[string]string{
		"cidr":          `{"ip":"198.51.100.0/24"}`,
		"hostname":      `{"ip":"evil.example"}`,
		"empty":         `{"ip":""}`,
		"unknown field": `{"ip":"198.51.100.7","ttl":9}`,
		"trailing data": `{"ip":"198.51.100.7"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := layer.Invoke(t.Context(), blockCall(args))
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
				t.Fatalf("result = %+v, want bad arguments", result)
			}
		})
	}
	if len(remote.added) != 0 {
		t.Fatal("something reached the remote list")
	}
}

// A public attacker address goes through, with the machine-readable comment
// the external expiry job prunes by, and the reason stripped to plain text.
func TestBlockIP_aPublicAddress_isAddedWithTheExpiryComment(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(),
		blockCall(`{"ip":"198.51.100.7","reason":"brute force\non /oauth2/token"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	if len(remote.added) != 1 || remote.added[0].IP != "198.51.100.7" {
		t.Fatalf("added = %+v", remote.added)
	}
	comment := remote.added[0].Comment
	if !strings.HasPrefix(comment, "fuseone:auto:2026-01-10T03:00:00Z") {
		t.Fatalf("comment = %q, want the fuseone:auto timestamp prefix", comment)
	}
	if strings.ContainsAny(comment, "\n\r") {
		t.Fatalf("comment carries control characters: %q", comment)
	}
	body := resultBody(t, layer, result)
	if body["blocked"] != "198.51.100.7" || body["alreadyBlocked"] != false {
		t.Fatalf("body = %+v", body)
	}
}

// Blocking an address that is already on the list succeeds without a second
// write: the outcome the run wanted is true, and the list does not grow
// duplicates that the expiry job would half-remove.
func TestBlockIP_anAddressAlreadyListed_succeedsWithoutASecondWrite(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z probe"},
	}}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), blockCall(`{"ip":"198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	if len(remote.added) != 0 {
		t.Fatal("a duplicate was written")
	}
	if body := resultBody(t, layer, result); body["alreadyBlocked"] != true {
		t.Fatalf("body = %+v", body)
	}
}

// The daily ceiling counts the list's own automatic entries for the day —
// the list is the state — and past it the connector refuses, so one misled
// night cannot empty the internet into the list.
func TestBlockIP_pastTheDailyCeiling_isRefused(t *testing.T) {
	t.Parallel()
	var items []CloudflareListItem
	for range defaultCloudflareMaxBlocksPerDay {
		items = append(items, CloudflareListItem{
			IP: "192.0.2.1", Comment: "fuseone:auto:2026-01-10T01:00:00Z x",
		})
	}
	// Yesterday's entries do not count against today.
	items = append(items, CloudflareListItem{
		IP: "192.0.2.2", Comment: "fuseone:auto:2026-01-09T23:00:00Z y",
	})
	remote := &fakeCloudflareList{items: items}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), blockCall(`{"ip":"198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
		t.Fatalf("result = %+v, want guard refusal", result)
	}
	if len(remote.added) != 0 {
		t.Fatal("the ceiling did not hold")
	}
}

// list_blocks returns the entries for the run to report; the read carries no
// arguments and tolerates none.
func TestListBlocks_returnsTheEntries(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{ID: "a", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z probe", CreatedOn: "2026-01-10T01:00:01Z"},
	}}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), engine.Call{
		Tool: "cloudflare.edge.list_blocks", RunID: "run-1", Seq: 8, Args: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	body := resultBody(t, layer, result)
	entries, _ := body["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", body)
	}
}

// Yesterday's entries do not consume today's ceiling: with room left today,
// the block goes through even though the list holds older automatic entries.
func TestBlockIP_underTodaysCeiling_yesterdayDoesNotCount(t *testing.T) {
	t.Parallel()
	var items []CloudflareListItem
	for range defaultCloudflareMaxBlocksPerDay - 1 {
		items = append(items, CloudflareListItem{
			IP: "192.0.2.1", Comment: "fuseone:auto:2026-01-10T01:00:00Z x",
		})
	}
	for range 5 {
		items = append(items, CloudflareListItem{
			IP: "192.0.2.2", Comment: "fuseone:auto:2026-01-09T23:00:00Z y",
		})
	}
	remote := &fakeCloudflareList{items: items}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), blockCall(`{"ip":"198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v, want the block to go through", result)
	}
	if len(remote.added) != 1 {
		t.Fatalf("added = %+v", remote.added)
	}
}

// A single IPv6 address is sent as itself — verified against the real API
// on 2026-10-08, which accepted it as a single entry. Widening to the /64
// would block more than was decided.
func TestBlockIP_aSingleIPv6_isSentAsItself(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(),
		blockCall(`{"ip":"2600:1f16:18f4:e000:521a:92ef:479:abbe"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	if len(remote.added) != 1 || remote.added[0].IP != "2600:1f16:18f4:e000:521a:92ef:479:abbe" {
		t.Fatalf("added = %+v, want the address itself", remote.added)
	}
}

// A covering CIDR an operator added by hand counts as already blocked: no
// duplicate is written inside it.
func TestBlockIP_anAddressInsideAHandAddedCIDR_isAlreadyBlocked(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{IP: "2600:1f16:18f4:e000::/64", Comment: "fuseone:auto:2026-01-10T01:00:00Z x"},
	}}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(),
		blockCall(`{"ip":"2600:1f16:18f4:e000::1234"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed || len(remote.added) != 0 {
		t.Fatalf("result = %+v, added = %+v", result, remote.added)
	}
	if body := resultBody(t, layer, result); body["alreadyBlocked"] != true {
		t.Fatalf("body = %+v", body)
	}
}

// An upstream refusal says which class it was — an authorization refusal, a
// missing list, or a plain failure — so the operator reading the run knows
// whether to fix the token, the ids or the network. Never the body, never
// the token.
func TestBlockIP_upstreamRefusals_areClassified(t *testing.T) {
	t.Parallel()
	for status, code := range map[int]string{
		401: CodeConnectorUpstreamAuth,
		403: CodeConnectorUpstreamAuth,
		404: CodeConnectorUpstreamNotFound,
		500: CodeConnectorUpstreamFailed,
	} {
		remote := &fakeCloudflareList{itemsErr: cloudflareRemoteError{status: status}}
		layer := cloudflareLayer(t, remote)
		result, err := layer.Invoke(t.Context(), blockCall(`{"ip":"198.51.100.7"}`))
		if err != nil {
			t.Fatalf("Invoke(%d): %v", status, err)
		}
		if !result.Failed || result.ErrorCode != code {
			t.Fatalf("status %d: result = %+v, want %s", status, result, code)
		}
	}
}

func (f *fakeCloudflareList) DeleteItem(
	_ context.Context, _ CloudflareConfig, _ SecretValue, id string,
) (string, error) {
	if f.deleteErr != nil {
		return "", f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return "op-del", nil
}

func unblockCall(args string) engine.Call {
	return engine.Call{
		Tool: "cloudflare.edge.unblock_ip", RunID: "run-1", Seq: 9,
		Args: []byte(args), DecidedBy: "usr_ana",
	}
}

// Unblock removes only what the connector itself wrote: an entry without the
// fuseone:auto prefix is a person's decision, and the agent never undoes a
// person. The refusal is the same clean guard code as the block side's.
func TestUnblockIP_aHandAddedEntry_isRefused(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{ID: "h1", IP: "198.51.100.7", Comment: "blocked by the SOC, keep"},
	}}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), unblockCall(`{"ip":"198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
		t.Fatalf("result = %+v, want guard refusal", result)
	}
	if len(remote.deleted) != 0 {
		t.Fatal("a person's entry was removed")
	}
}

// An automatic entry is removed by its id, and the result names what fell.
func TestUnblockIP_anAutomaticEntry_isRemoved(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{ID: "a1", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z probe"},
	}}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(),
		unblockCall(`{"ip":"198.51.100.7","reason":"false positive, partner NAT"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	if len(remote.deleted) != 1 || remote.deleted[0] != "a1" {
		t.Fatalf("deleted = %+v, want the entry's id", remote.deleted)
	}
	body := resultBody(t, layer, result)
	if body["unblocked"] != "198.51.100.7" || body["alreadyAbsent"] != false ||
		body["operationId"] != "op-del" {
		t.Fatalf("body = %+v", body)
	}
	// The removed entry's comment is stored text that once came through a
	// model; it must not re-enter a model's context through the result.
	if _, leaked := body["removedComment"]; leaked {
		t.Fatal("the stored comment re-entered the result")
	}
}

// An address that is not on the list is already in the desired state: success
// that says so, and nothing is deleted.
func TestUnblockIP_anAbsentAddress_succeedsSayingSo(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), unblockCall(`{"ip":"198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	if len(remote.deleted) != 0 {
		t.Fatal("something was deleted")
	}
	if body := resultBody(t, layer, result); body["alreadyAbsent"] != true {
		t.Fatalf("body = %+v", body)
	}
}

// A covering hand-added CIDR is not "the block for this address": the exact
// automatic entry is what unblock may touch, and a person's wider range
// stays untouched and still covers — the result must say the address
// remains blocked by it.
func TestUnblockIP_underAHandAddedCIDR_refusesAndSaysWhy(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{ID: "h1", IP: "198.51.100.0/24", Comment: "SOC: permanent range"},
	}}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), unblockCall(`{"ip":"198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
		t.Fatalf("result = %+v, want guard refusal", result)
	}
	if len(remote.deleted) != 0 {
		t.Fatal("a person's range was removed")
	}
}

// Bad arguments on the unblock side: not literal, unknown fields.
func TestUnblockIP_badArguments(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{}
	layer := cloudflareLayer(t, remote)
	for name, args := range map[string]string{
		"cidr":          `{"ip":"198.51.100.0/24"}`,
		"unknown field": `{"ip":"198.51.100.7","force":true}`,
		"empty":         `{"ip":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := layer.Invoke(t.Context(), unblockCall(args))
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
				t.Fatalf("result = %+v, want bad arguments", result)
			}
		})
	}
}

// A failed upstream delete is a failed call with its class — never a success
// that left the address blocked while the run reports it gone.
func TestUnblockIP_aFailedUpstreamDelete_failsWithItsClass(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{
		items: []CloudflareListItem{
			{ID: "a1", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z x"},
		},
		deleteErr: cloudflareRemoteError{status: 403},
	}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), unblockCall(`{"ip":"198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != CodeConnectorUpstreamAuth {
		t.Fatalf("result = %+v, want the auth class", result)
	}
}

// The IPv4-mapped IPv6 form names the same address: it removes the exact
// IPv4 entry, pinning the Unmap on the unblock side too.
func TestUnblockIP_anIPv4MappedForm_matchesTheIPv4Entry(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{ID: "a1", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z x"},
	}}
	layer := cloudflareLayer(t, remote)
	result, err := layer.Invoke(t.Context(), unblockCall(`{"ip":"::ffff:198.51.100.7"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed || len(remote.deleted) != 1 || remote.deleted[0] != "a1" {
		t.Fatalf("result = %+v deleted = %+v", result, remote.deleted)
	}
}

// Both at once: the connector's own entry AND a person's covering range.
// The person's decision wins whatever the list order — the address must
// stay blocked, so nothing is deleted.
func TestUnblockIP_autoEntryInsideAPersonsRange_isRefusedWholesale(t *testing.T) {
	t.Parallel()
	for name, items := range map[string][]CloudflareListItem{
		"range first": {
			{ID: "h1", IP: "198.51.100.0/24", Comment: "SOC: permanent"},
			{ID: "a1", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z x"},
		},
		"range last": {
			{ID: "a1", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z x"},
			{ID: "h1", IP: "198.51.100.0/24", Comment: "SOC: permanent"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			remote := &fakeCloudflareList{items: items}
			layer := cloudflareLayer(t, remote)
			result, err := layer.Invoke(t.Context(), unblockCall(`{"ip":"198.51.100.7"}`))
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
				t.Fatalf("result = %+v, want refusal", result)
			}
			if len(remote.deleted) != 0 {
				t.Fatal("something was deleted under a person's range")
			}
		})
	}
}

// Removing protection always carries a decision's identity: a call that no
// click and no mandate released is refused before anything is read, whatever
// an allow policy said. This is the closure of the review's mass-removal
// finding — each unblock is named, counted and capped by its decision path.
func TestUnblockIP_withoutADecidedIdentity_isRefused(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{ID: "a1", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z x"},
	}}
	layer := cloudflareLayer(t, remote)
	call := unblockCall(`{"ip":"198.51.100.7"}`)
	call.DecidedBy = ""
	result, err := layer.Invoke(t.Context(), call)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != CodeConnectorNeedsDecision {
		t.Fatalf("result = %+v, want the needs-decision refusal", result)
	}
	if len(remote.deleted) != 0 {
		t.Fatal("something was removed without a decision")
	}
}

// Stored comments are remote text that once came through a model: the
// results that echo them carry the untrusted label, so checkTaint sees any
// write they later steer. Cross-run prompt injection must not launder its
// taint through the block list.
func TestCloudflareResults_thatEchoComments_areLabeledUntrusted(t *testing.T) {
	t.Parallel()
	remote := &fakeCloudflareList{items: []CloudflareListItem{
		{ID: "a1", IP: "198.51.100.7", Comment: "fuseone:auto:2026-01-10T01:00:00Z x"},
	}}
	layer := cloudflareLayer(t, remote)

	listed, err := layer.Invoke(t.Context(), engine.Call{
		Tool: "cloudflare.edge.list_blocks", RunID: "run-1", Seq: 8, Args: []byte(`{}`),
	})
	if err != nil || listed.Failed {
		t.Fatalf("list: %+v %v", listed, err)
	}
	if !listed.Labels.Has(domain.LabelUntrusted) {
		t.Fatal("list_blocks echoes comments without the untrusted label")
	}

	already, err := layer.Invoke(t.Context(), blockCall(`{"ip":"198.51.100.7"}`))
	if err != nil || already.Failed {
		t.Fatalf("alreadyBlocked: %+v %v", already, err)
	}
	if !already.Labels.Has(domain.LabelUntrusted) {
		t.Fatal("the alreadyBlocked echo carries no untrusted label")
	}
}
