#!/bin/sh
# factory-mcp-smoke.sh IMAGE — the image starts as the sidecar does and answers
# /healthz.
#
# Run against the built image, not the binary: a missing ENTRYPOINT, a binary
# built for the wrong platform or a base without certificates passes every Go
# test and fails here. The board values are placeholders, shaped like the real
# ones and naming no board; pass-through mode holds no credential, so nothing
# here is a secret either.
set -eu

image=${1:?usage: factory-mcp-smoke.sh IMAGE}
name=factory-mcp-smoke-$$
port=18091

docker run -d --name "$name" --network host \
  -e FACTORY_MCP_ADDR="127.0.0.1:$port" \
  -e FACTORY_MCP_AUTH=github-app-token \
  -e FACTORY_ROLE_BOTS=triage=101,developer=103 \
  -e FACTORY_REPOSITORY=example/repository \
  -e FACTORY_PROJECT_ID=PVT_placeholder \
  -e FACTORY_STATUS_FIELD_ID=PVTSSF_status \
  -e FACTORY_LANES=Triagem=a,Preparado=b,Doing=c,Travado=d,Done=e \
  -e FACTORY_WORKTYPE_FIELD_ID=PVTSSF_worktype \
  -e FACTORY_WORKTYPES=bug=f,feature=g,chore=h,docs=i \
  -e FACTORY_SIZE_FIELD_ID=PVTSSF_size \
  -e FACTORY_SIZES=S=j,M=k,L=l \
  "$image" >/dev/null
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT

for _ in $(seq 1 30); do
  if body=$(curl -fsS "http://127.0.0.1:$port/healthz" 2>/dev/null); then
    if [ "$body" = "ok" ]; then
      echo "factory-mcp image answers /healthz"
      exit 0
    fi
  fi
  sleep 1
done

echo "factory-mcp image did not answer /healthz within 30s; its log:" >&2
docker logs "$name" >&2 || true
exit 1
