---
title: Manual and design notes
description: Rendered product manual, design record and source-of-truth notes.
---

The public site is the navigable entry point. Source Markdown remains in the
repository, but the pages below are rendered here so readers do not have to open
raw repository files for normal product documentation.

## In-product manual

The manual is embedded into the binary from `docs/manual`. The same pages are
used by the console and reviewed in pull requests.

- [English manual](../../manual/en-us/)
- [Manual em portugues](../../manual/pt-br/)

## Product and operation docs

- [PRD-001: product requirements](../../design/prd-001-fuseone-agents/)
- [DP-001: data protection](../../design/dp-001-data-protection/)
- [OP-001: running an installation](../../design/op-001-running-an-installation/)
- [Helm chart reference](../helm-chart/)

## Design notes

- [NT-001: integration boundary and execution model](../../design/nt-001-integration-boundary-and-execution-model/)
- [NT-002: remaining work](../../design/nt-002-remaining-work/)
- [NT-003: conversational authoring](../../design/nt-003-conversational-authoring/)
- [NT-004: ledger volume and paging](../../design/nt-004-ledger-volume-and-paging/)
- [NT-005: interaction channels](../../design/nt-005-interaction-channels/)
- [NT-006: evaluating agents](../../design/nt-006-evaluating-agents/)
- [NT-007: drawing a process](../../design/nt-007-drawing-a-process/)
- [NT-008: catalogue by shape](../../design/nt-008-a-catalogue-by-shape/)
- [NT-009: governed connectors](../../design/nt-009-governed-connectors/)

## Source of truth

The rendered pages are generated during the docs build. The editable source
still lives in
[the repository docs directory](https://github.com/fuseone-io/fuseone-agents/tree/main/docs).
