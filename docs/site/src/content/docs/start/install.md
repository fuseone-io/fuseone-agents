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

The detailed install and operations guide lives in the repository:

- [Helm chart README](https://github.com/fuseone-io/fuseone-agents/blob/main/deploy/helm/fuseone-agents/README.md)
- [OP-001: running an installation](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/OP-001-running-an-installation.md)
- [DP-001: data protection](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/DP-001-data-protection.md)

## Local development

For local development, the repository includes a full dev stack:

```sh
make dev
make check
make check-pg
```

`make dev` starts the database, API, worker, console and local stand-ins for
the model provider and MCP server.
