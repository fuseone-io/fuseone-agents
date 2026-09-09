---
title: Slack and channels
summary: How events arrive, who authorises a run, and why mentions, watched messages and approvals are different paths.
section: integrations
tags: slack, channel, socket mode, event subscriptions, approval, runAs, thread
order: 4
---

## A channel has different paths

Slack uses similar words for different things. Configure each one in the right
place.

| Path | What it uses | What it is for |
|---|---|---|
| Socket Mode | App-level token `xapp-...` | Receive events over an outbound WebSocket, with no public URL |
| Event Subscriptions | `/hooks/channel/<name>/slack/events` | Receive mentions and messages |
| Interactivity | `/hooks/channel/<name>/slack` | Receive button clicks, such as approvals |
| Bot API | Bot token `xoxb-...` | List channels, post messages and reply in threads |
| HTTP verification | Signing secret | Verify that an HTTP call came from Slack |

The signing secret does not replace the app token. The app token does not
replace the bot token. In Socket Mode, Slack delivers events through
`xapp-...`, but posting and listing channels still use `xoxb-...`.

## HTTP callback

Use HTTP when Slack can reach a public URL for the installation.

In the Slack App:

1. In Event Subscriptions, enter
   `https://<host>/hooks/channel/<name>/slack/events`.
2. Subscribe to `app_mention` for mentions.
3. Subscribe to `message.channels` if you want watched messages in public
   channels.
4. In Interactivity & Shortcuts, enter
   `https://<host>/hooks/channel/<name>/slack`.
5. Invite the bot to the channel.

If Slack says the URL did not answer the `challenge` and the serve log shows
`POST /hooks/channel/<name>/slack status=400`, the events URL was configured
on the button endpoint. The events endpoint ends in `/events`.

## Socket Mode

Use Socket Mode when the installation has no public inbound URL. The worker
opens an outbound connection to Slack.

You still need:

- app token `xapp-...` with `connections:write`;
- bot token `xoxb-...`;
- bot invited to the channel;
- events subscribed in the Slack App.

Approval buttons still need the HTTP Interactivity path. If Slack cannot call
`/hooks/channel/<name>/slack`, the platform should not show approval buttons
in Slack.

## Mentions

A conversation can name the agent it starts. With an agent chosen, mentioning
the bot needs no name and the whole sentence is the question:

```text
@FuseOneAgent investigate this alert
```

In that conversation **no other agent starts from a mention**. Naming a
different one is refused, and the refusal says which agent this conversation
starts, rather than running another agent on the asker's sentence.

With no agent chosen — the "none — the message names the agent" option — the
message has to start with the agent id or name, and any agent published in the
conversation's scope can be started:

```text
@FuseOneAgent troubleshooting-sre investigate this alert
```

Either way the conversation decides the scope and the text does not choose
company or area. The agent starts only if the published version declares a
conversation trigger and exists in that scope.

The chosen agent **selects; it does not authorise**. The run still acts on
behalf of the person whose Slack account is linked to a platform user, and an
unlinked account is still refused.

Mentions start agents only in "mentions only" and "both". In a conversation set
to "watched messages", mentioning the bot is refused with the reason — the mode
says only the configured sources start agents, and that holds for both delivery
paths.

A mention with no words starts nothing: `@FuseOneAgent` on its own is refused
with a sentence saying what to do. The exception is a mention that arrives with
something to work on — a thread the platform posted a run into, or a thread
whose earlier messages were actually read because "include thread context" is
on. Being in a thread is not enough: a thread nobody read leaves the agent as
empty-handed as a bare mention.

The same rule covers watched messages. When a message has no `text` of its own,
the platform reads the words out of its `blocks` and `attachments` — which is
how most alerting integrations post — and refuses only when it finds none. The
delivery is still recorded in full, so the payload is there to check.

## Watched messages

Watched messages are for messages from a known system, such as Alertmanager or
Grafana OnCall. Authority does not come from the message. It comes from the
conversation configuration:

- which agent to start;
- which principal is `run as`;
- which Slack user ids, bot ids or app ids may trigger.

Use stable ids, not display names. Display names change; ids are what Slack
signs.

## Thread context

If "include thread context" is enabled, earlier messages in the thread enter
as untrusted input. This helps an agent read the original alert when the
mention happens later.

That option can include messages written by other people. They may be sent to
the configured model provider. Enable it only in channels where that
consequence has been accepted.

## Diagnostic checklist

If the agent did not start:

1. Does the log show `an ask arrived`? If not, the event did not reach FuseOne.
2. Is the conversation in the right mode: mention, watched messages or both?
2b. Did the mention name an agent other than the one chosen on the conversation?
3. Does the published agent have a conversation trigger?
4. Is the agent published in the same scope as the conversation?
5. For watched messages, is the Slack source id allowed?
6. Does `run as` exist and have a grant in the scope?
7. In Socket Mode, is the app token saved and is the worker connected?

## Telling the people who may decide, privately

A conversation that is told about parked runs can also send the same request as
a direct message from the bot to the people who may decide it. Turn it on under
**Integrations → Channels → the conversation → "Also tell the people who may
decide, privately"**.

The recipients are the people holding **Approver** in a scope that contains the
run — not administrators, who may decide and would be told about every parked
run in the whole installation. Each needs a linked Slack account; anybody
unlinked is simply not reached.

The direct message **addresses and does not authorise**. The button is checked
against the run's own scope wherever it is pressed, so somebody without the
grant is refused in a private message exactly as they would be in a channel.

It also **does not replace the channel**. The channel card goes first, and a
conversation that could not be told is not answered with a private message
instead — the ledger records every decision, but the visibility that makes
somebody notice a run has been waiting two hours is in the channel.

It needs the Slack app's **`im:write`** scope. Without it the direct message is
refused, the failure is recorded, and the channel is still told.

If more than twenty people may decide, nobody is messaged privately and the
reason is recorded. Telling an arbitrary twenty of a hundred is worse than
telling none: nobody can say who was meant to be asked.

## Cards that have been answered

Once somebody decides, every card about that step is rewritten to say what
happened and who decided it, and the buttons go. This covers the channel card
too, so an approval decided in the console no longer leaves live buttons
behind.

A card that cannot be rewritten — the message was deleted, the conversation is
gone — is marked closed anyway, so it does not consume the sweep for ever.
