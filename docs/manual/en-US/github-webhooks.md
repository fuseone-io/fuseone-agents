---
title: GitHub webhooks
summary: Start an agent when GitHub sends an event — a pull request opened, an issue labelled — with the delivery signed by GitHub and filtered by the trigger.
section: integrations
tags: github, webhook, trigger, signature, events, delivery
order: 23
---

# GitHub webhooks

An agent can be started by GitHub itself: a pull request opened, an issue
labelled, a card moved. GitHub does not send a secret the way an ordinary
webhook sender does. It **signs** each delivery with one, names the delivery in
its own header, and sends every event the hook subscribes to. A webhook
trigger that says `from: github` is checked that way.

## Declare the trigger

In the agent's definition — or in the console, under **Triggers**, choosing
**Sent by: GitHub** — a webhook names where its deliveries come from and which
of them start a run:

```yaml
triggers:
  - type: webhook
    path: github/reviewer
    from: github
    events: [pull_request.opened, pull_request.synchronize, issues.labeled]
```

- `event` takes every action of the event: `issues` starts a run whether an
  issue was opened, edited or labelled.
- `event.action` takes only that action: `pull_request.labeled`.
- At least one event is required. Without the list every delivery GitHub sends
  would start a run, which on a busy repository is a bill.
- `ping` is not listed: GitHub sends it once, when the hook is created, and the
  platform answers it without starting anything.

Publishing refuses a GitHub webhook with no events, a name GitHub never sends,
and events on a webhook that does not say `from: github`.

## Generate the key

A declared path refuses everything until it has a key. On the agent's page,
**Webhooks** lists the path marked **GitHub**; **Generate a key** shows it
once.

The key of a GitHub webhook is kept sealed by the installation's master key
(`FUSEONE_MASTER_KEY`), because the platform has to read it back to check each
signature. Without a master key the console refuses to generate one, and the
webhook stays closed. An ordinary webhook keeps only a hash of its key.

The key is never accepted in `X-FuseOne-Secret`: it signs, it is not sent.

## Configure the webhook in GitHub

In the repository's or organisation's **Settings → Webhooks**, or on a GitHub
App's webhook settings:

| Field | Value |
|---|---|
| Payload URL | The installation's address followed by `/hooks/` and the path, for example `https://agents.example.com/hooks/github/reviewer` |
| Content type | `application/json` |
| Secret | The key the console showed |
| Events | The events the trigger lists, or more: the platform ignores the rest |

A form-encoded body is refused with the reason: the run's input has to be the
JSON GitHub would otherwise send.

## What each delivery gets back

| The delivery | Answer | Run |
|---|---|---|
| Signed, an event the trigger lists | 202 | Opened |
| The same `X-GitHub-Delivery` again | 200, naming the same run | None new |
| Signed, an event or action not listed | 200 | None |
| `ping` | 200 | None |
| No signature, a wrong one, a path with no key, an unknown path, or a body over 1 MiB | 401, the same words for all | None |
| Signed with no `X-GitHub-Delivery`, or a body that is not JSON | 400 or 415, with the reason | None |

A non-2xx answer makes GitHub retry and mark the hook as failing, which is why
deliveries the trigger does not want are acknowledged rather than refused.
Everything that did not prove itself answers the same way, so a caller probing
paths learns nothing about which exist.

## What the run receives

The run's input is the delivery, wrapped so the agent knows what it was —
GitHub's body does not carry its own event name:

```json
{
  "event": "pull_request",
  "action": "opened",
  "delivery": "72d3162e-cc78-11e3-81ab-4c9367dc0958",
  "payload": { "…": "the body GitHub sent" }
}
```

The run starts **untrusted**: the signature proves GitHub sent the delivery,
not that the issue or pull request text inside it is safe to act on. On a
public repository anybody can write that text, so the Gate treats what the
agent does after reading it accordingly — see
[Data labels and barriers](data-barriers.md).
