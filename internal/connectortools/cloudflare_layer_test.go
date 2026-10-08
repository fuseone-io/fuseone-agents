package connectortools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/engine"
)

type fakeCloudflareList struct {
	items    []CloudflareListItem
	itemsErr error
	added    []CloudflareListItem
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
