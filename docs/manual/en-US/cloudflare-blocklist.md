---
title: Cloudflare block list
summary: Let an agent block an attacker at the Cloudflare edge, inside guards that live in code and blocks that expire by design.
section: integrations
tags: cloudflare, firewall, block list, security, connector
order: 19
---

# Cloudflare block list

The connector gives an agent one narrow power: add an attacker's address to
one Cloudflare IP list that a firewall rule already blocks at the edge. It
writes that list and nothing else — never rules, zones or DNS — and the
guards that bound it live in the platform's code, not in any prompt.

## The design around it

The connector is one piece of a three-part design; the other two live in the
operator's own infrastructure:

1. **A firewall rule** in the Cloudflare account with the expression
   `ip.src in $your_list → Block`. The rule is managed wherever the operator
   manages Cloudflare (Terraform, dashboard); the connector never touches it.
2. **This connector**, which adds entries to the list the rule reads.
3. **An expiry job** the operator runs (a cron anywhere), which deletes
   entries whose comment says they are old enough. Blocks made here are
   temporary by design: a mistaken one undoes itself, and a real attacker who
   returns is simply blocked again.

## The comment contract

Every entry the connector writes carries the comment

```
fuseone:auto:<RFC3339 timestamp> <reason>
```

The timestamp is what the expiry job trusts; the reason is the agent's own
words, flattened to one line and capped. An entry without the `fuseone:auto:`
prefix was added by a person, and neither this connector nor the expiry job
should ever touch it.

## What block_ip refuses, in code

- Anything that is not one literal IP address: a CIDR, a hostname, junk.
- Private, loopback, link-local, multicast and unspecified addresses —
  unconditionally; no configuration reopens them.
- The instance's **protected ranges**: CIDRs the operator lists for the
  installation's own egress and for anonymizer ranges whose addresses are
  shared by many real clients.
- The address when the day's automatic entries already reached the
  instance's **daily ceiling** — counted from the list itself.

An address already on the list returns success without a second write, so the
list never grows duplicates. Every refusal is a clean failed tool call with a
code the run records.

`unblock_ip` is the other direction, under the strictest guard of all: it
removes **only entries the connector itself wrote** — the `fuseone:auto:`
comment. A person's entry, or a person's wider range covering the address,
is a person's decision: the call is refused and the address stays blocked.
An address not on the list is already in the desired state and succeeds
saying so.

## Configuring an instance

Integrations → Connectors → New Cloudflare block list. Account ID and List ID
name the one list; the API token is sealed on save and never returned — the
card only says whether one is stored. The token needs exactly one Cloudflare
permission: editing account filter lists. Give it nothing else, and the
instruction "write only the list" stops being a promise.

## Governance

`cloudflare.<instance>.block_ip` is a write effect. By default no approval
card is raised — the use case is the night shift no human is awake for — but
a policy can require one, and a policy with reach "agents" can hand the tool
to exactly one agent. The run's ledger records every block and every refusal
either way.
