---
title: Governed Gravitee tickets
summary: Turn one Slack support thread into an inspected, approved and recoverable Gravitee subscription acceptance.
section: integrations
tags: gravitee, slack, ticket, api key, approval, vault
order: 18
---

## What this flow does

A linked developer opens a matching root message in a configured Slack support
room. FuseOne keeps that thread as one ticket, lets the requester correct it,
inspects the pending Gravitee subscription and asks a named manager to approve
the exact acceptance. After one valid decision, it accepts the subscription
once and replies in the original thread.

The reply names the subscription and expiration and reminds the requester to
store and rotate the credential safely. It **never contains the API key** or
remote display names. The requester retrieves the key from Gravitee through
the organization's protected path.

## Before configuring the room

1. Configure an enabled Gravitee connector in the same company and area as the
   support room. Bind exactly one API, an organization, an environment and the
   allowed expiration range.
2. Bind the connector to a Vault-backed credential with only subscription read
   and accept permissions for that remote scope. FuseOne resolves the secret
   internally; the model and Slack never receive it.
3. Publish an agent with a conversation trigger and access to that connector.
   Its instructions should collect a subscription id, reason and requested
   expiration, inspect first, and call only the registered acceptance tool.
4. Link the requester's and approver's Slack accounts to FuseOne people. Give
   the approver a role carrying `approval:act` in a scope that contains the
   ticket.
5. Give the Slack app `message.channels`, invite it to the support room,
   configure Interactivity for approval buttons, and grant `im:write` for
   direct messages.

## Configure the conversation

Under **Integrations -> Channels**, add or edit the support room and choose
**Governed tickets**. Select its scope and agent, choose the service principal
used as `run as`, enter the exact addressing source as `bot:<id>` or
`app:<id>`, and add narrow RE2 patterns that identify supported requests.

`run as` is the execution principal; it is not the requester. The linked human
who wrote the root remains `RequestedBy`, and the configured bot or app merely
chooses who is notified. None of those identities substitutes for the person
who presses Approve.

## Validate safely

1. Create a pending test subscription in the one configured Gravitee API.
2. Post a matching root request from a linked Slack account and include its
   subscription id and desired expiration.
3. Confirm a ticket run opens in the room's scope and that the original text is
   marked untrusted.
4. Have the configured addressing bot or app mention one real approver in the
   thread. Confirm the card appears under the root and in that person's DM.
5. Open the approval and compare every evidence field with Gravitee. Change the
   remote subscription before approving once; FuseOne must refuse the stale
   snapshot without sending the acceptance.
6. Repeat with a fresh subscription and approve. Confirm Gravitee records one
   acceptance and the Slack thread receives the safe result without a key.
7. Remove the approver's grant or account binding and repeat. The DM must not be
   delivered and no named recipient may gain authority from the ticket.

If the process stops after Gravitee may have accepted the request, do not submit
it manually first. The recovery worker reconciles the recorded attempt against
the subscription and never repeats an ambiguous POST automatically. If it
cannot prove the outcome inside the bounded window, the thread asks for manual
verification while FuseOne retains the execution claim and continues GET-only
observation. A later confirmed state is posted in the same thread.
