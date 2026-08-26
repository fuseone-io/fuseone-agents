---
title: Install
description: The installation shape and the first links operators should read.
---

FuseOne Agents is installed into the customer's own environment. The shipping
shape is one binary, one PostgreSQL database and one Helm chart.

```sh
helm install agents oci://ghcr.io/fuseone-io/charts/fuseone-agents \
  --namespace fuseone --create-namespace \
  --set secret.existingSecret=fuseone-agents \
  --set baseUrl=https://agents.example.com
```

Images are published for `linux/amd64` and `linux/arm64` and signed with
keyless Sigstore identity. Verify the image for the version you intend to run:

```sh
cosign verify ghcr.io/fuseone-io/fuseone-agents:<version> \
  --certificate-identity-regexp '^https://github.com/fuseone-io/fuseone-agents/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Operator reference

The detailed install and operations guides are rendered in this site:

- [Helm chart reference](../../reference/helm-chart/)
- [OP-001: running an installation](../../design/op-001-running-an-installation/)
- [DP-001: data protection](../../design/dp-001-data-protection/)

## Local development

For local development, the repository includes a full dev stack:

```sh
make dev
make check
make check-pg
```

`make dev` starts the database, API, worker, console and local stand-ins for
the model provider and MCP server.
