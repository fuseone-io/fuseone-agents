---
title: Companies, areas and roles
summary: Where an agent lives, who reaches what, and why administering identity needs the installation scope.
section: governance
tags: scope, company, area, role, permission, administrator, grant
order: 9
---

## Two dimensions: where and who

**Company and area** say *where* something lives. An agent is published into a company/area pair, and that is where it runs, spends and is audited.

**Role** says *what* a person may do. And a role never stands alone — it is always granted **inside a scope**.

The same person can be an author in `cora/platform` and only an approver in `cora/finance`. That is not an exception; it is the design.

## An area is scoped by its company

`platform` inside `cora` and `platform` inside `default` are **different areas**, despite the identical name.

This matters when publishing: you choose the pair, not just the area. An agent published into the wrong pair runs with somewhere else's permissions.

## The roles

| Role | For |
|---|---|
| **Author** | Writes, rehearses and publishes agents |
| **Approver** | Decides the calls that stopped |
| **Curator** | Classifies tools and looks after the surface |
| **Administrator** | Everything, at the installation scope |

There is no inheritance and no wildcard: **reading one row tells you everything that role can do**. An author does not classify a tool; an approver does not publish an agent.

## The installation scope

There is one scope above all others: company `*` with an empty area. It reaches every company and area, including ones created tomorrow.

**Administrator granted there** is the installation's administrator — bootstrap, identity, integrations, branding, global ceilings.

**Administrator granted in an ordinary company or area** stays scoped: they may do everything, but only there.

## Why identity needs the installation

Creating people, changing grants and setting a local password all require `identity:write` **at the installation scope**.

The reason is concrete: whoever administers identity can grant administrator to anybody, including themselves, anywhere. If that permission worked inside an area, whoever administered that area could mint installation-wide administrators — from a scope nobody thinks of as privileged.

## Grants that come from the identity provider

When sign-in goes through an external provider, a grant can come from a **group** it asserts. Those are **re-derived on every sign-in**.

The practical consequence: **revoking that grant on the screen lasts until the person signs in again.** What has to change is the group, in the provider. The People screen marks each grant's origin — `local`, `provider` or `mixed` — precisely so this is not discovered on a Monday.

## Use cases

### A team looks after its own agent

Author in `company/team-area`. They write, rehearse and publish there, and reach nothing in another area.

### Somebody approves without being able to edit

Approver in the area. They see the runs and decide what stopped, and cannot change the instruction to get around their own refusal.

### Whoever sets the platform up

Administrator at the installation scope — company `*`, empty area. One grant, not four roles assembled by hand.

### A team that also classifies its own tools

Author **and** curator in the same area. Classifying is a judgement about what a tool does to the world, and it is reasonable for that to sit with whoever knows the domain — as long as it is recorded that it did.

What each stage permits is in [Draft, copilot and autonomous](autonomy-stages.md).
