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

## Governed ticket conversations

Choose **Governed tickets** when each matching root message is a support ticket
and its thread is the ticket's history. This is different from a watched
message: the writer must have a linked FuseOne account, becomes the immutable
requester, and only that person can revise the requested fields.

The conversation asks for an agent, a `run as` principal, one exact bot or app
allowed to name recipients, and one to eight RE2 admission patterns. Patterns
are matched only against bounded root-message text. Replies are found by the
stored Slack origin and are never classified again from their prose.

The addressing source may reply with Slack mentions to choose who receives the
approval. Those names grant nothing: they are intersected with people who can
currently perform `approval:act` in the ticket scope, and the button checks the
permission again. The approval card is posted under the original root and the
same card is sent by DM only to the surviving named recipients.

### When a bot writes the root

In a help channel with a form, the root message belongs to the bot that posts
the form, and the team that owns the request is named later, in the thread.
Choose **"How a ticket opens" → A mark in the thread**.

That policy asks for one thing more: the **source that posts the root**, the
exact id of the form's bot or app. Patterns stop being matched against the root
and are matched against replies instead — the first one that matches opens the
ticket. Who may mark is the configured addressing source or any person in the
channel; another bot is ignored, so an integration that echoes the mark cannot
open work.

The ticket keeps the root's identity: the request is the root's text, the
requester is the person mentioned on its first line, and every later reply lands
on that same ticket. The form has to render the requester there: a mention below
the first line is text the filer typed and never names who asked. The mention is evidence and not authority — it is resolved against
the accounts linked on that connection, and a thread whose requester does not
resolve never becomes a ticket. If the root came from a source other than the
configured one, the mark opens nothing.

Reading the root is one Slack call at opening time: the app needs the channel
history scope, the same one thread context uses. Without it the thread waits
and no ticket opens.

### The room where the answer is reviewed

When this conversation's tickets should be worked away from the person who
asked, point **Review room** at another conversation in the same workspace — by
convention, a channel only the team reads.

When the ticket opens, the bot opens a thread there saying a request arrived and
linking the original thread. Everything happens in it from then on: the decision
card is posted in that thread and nowhere else, neither in the ticket thread nor
in another conversation covering the scope. Whoever holds Approver in the scope
corrects it by replying in the room — any of them, not only whoever was
addressed — and each correction is a revision: the pending approval is cancelled
and the agent writes the answer again. Once approved, the text is published in
the ticket thread and the room is told it was.

In the ticket thread, whatever the requester writes is kept in the request and
starts nothing: what regenerates an answer is a correction in the room. While a
decision is pending it is recorded and goes no further — a revision written then
would supersede the card somebody is reading with nothing regenerating to
replace it. People who cannot decide can talk in the room without starting
anything, and naming recipients by mention does not apply here: the room is the
address.

Publishing an answer settles that revision, not the ticket. The approved
revision keeps the record of what was published, and the ticket goes on
accepting corrections: the next thing the requester says is about the same
request, in the same thread. A ticket ends when somebody holding Approver in the scope reacts to the request
with one of the emoji configured under **Emoji that closes the ticket**. A
reaction from anybody else is ignored in silence, a reaction taken away reopens
nothing, and an answer waiting for approval when the ticket closes is cancelled
with a line in the room saying it was not published. Closed, the ticket takes
nothing more — no correction in the room, no context in the thread.
Until it closes it still counts against the area's cap of open tickets.

For the agent to propose the answer, declare the `$fuseone.ticket.answer` tool
on it. It is a write effect: the agent calls it with the text, the Gate stops
the run, and the card in the room carries exactly those words. Once approved,
the text is sealed as the ticket's outcome and published in the ticket thread,
and the room is told. If the agent rewrites the answer after the card went out,
publishing is refused — the approval is about the text a person read. An
approved answer completes the ticket.

While the room's thread does not exist, cards wait: nothing falls back to the
ticket thread for want of somewhere to go. The bot has to be in the room, or the
ticket sits there — the worker records the failure and tries again.

For the complete Gravitee setup and validation sequence, see
[Governed Gravitee tickets](gravitee-tickets.md).

## A room for the whole installation

A conversation receives the runs of the scope it was configured in, and one
configured at a company covers every area in it. What is left over is an area
nobody pointed at a channel at all: a run parked there produces no card and no
private message, and waits for somebody to open the console.

Under **Integrations → Channels → new conversation**, the context **"The whole
installation"** answers that. It hears about a run in any company, with the
same options as any other conversation — including telling the people who may
decide privately, which still reaches whoever holds Approver in that run's
scope, not whoever holds a grant at the installation.

It **starts nothing**. Mentioning the bot there is refused, saying the room only
reports, and no watched-message automation runs from it. A scope that contains
everything is the right answer for *hearing* and the wrong one for *asking*: a
room that hears every company is a reasonable thing to configure, and one that
can start an agent in every company is something else entirely.

That is why the context is offered only to whoever governs the installation —
whoever holds **Company configurer granted at the installation scope**.
Configuring where one area reports is an ordinary scoped act; deciding that one
room hears every company is the authority above them all. Deleting it needs the
same.

Begin with **"what to announce"**. The default is *parked*, *failed* and
*drifted*, and at this scope that means every stop, every failure and every
drift in every company — such a room's volume is the sum of all the others.
Narrow it before saving.

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
