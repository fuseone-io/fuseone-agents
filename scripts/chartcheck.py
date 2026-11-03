"""What renders correctly and still will not run.

kubeconform proves a manifest has the right shape. It does not prove a
volumeMount resolves to a volume that exists, and that is how a Deployment the
API server refuses passed CI, `helm lint` and review — and reached two
published versions.

Every combination, because the defect only appeared in one of them: the
worker's `/tmp` volume was rendered inside the conditional for a specs
ConfigMap, so it was present in the case somebody would think to check and
absent in the default.
"""

import json
import subprocess
import sys

import yaml

CHART = "deploy/helm/fuseone-agents"

# What an operator puts in worker.extraContainers. Deliberately nothing the
# chart knows about: the hook carries the operator's container verbatim, and a
# chart that recognised its contents would have stopped being generic.
SIDECAR = {
    "name": "sidecar",
    "image": "example.com/sidecar:1",
    "env": [{"name": "LISTEN", "value": "127.0.0.1:9000"}],
    "readinessProbe": {"exec": {"command": ["/sidecar", "healthcheck"]}},
}
HOOK = ["--set-json", "worker.extraContainers=" + json.dumps([SIDECAR])]

COMBINATIONS = [
    [],
    ["--set", "worker.specs.configMap=specs"],
    ["--set", "ingress.enabled=true", "--set", "ingress.host=a.example.com"],
    ["--set", "gateway.enabled=true", "--set", "gateway.parentRefs[0].name=g"],
    ["--set", "networkPolicy.enabled=true"],
    ["--set", "worker.stdioEgress.networkPolicy.enforced=true"],
    ["--set", "networkPolicy.enabled=true", "--set", "worker.stdioEgress.networkPolicy.enforced=true"],
    ["--set", "topologySpread.enabled=false"],
    HOOK,
    [*HOOK, "--set", "worker.specs.configMap=specs", "--set", "networkPolicy.enabled=true"],
]
CARRIES_PODS = ("Deployment", "Job", "StatefulSet", "DaemonSet")


def mounts_resolve(doc):
    spec = doc["spec"]["template"]["spec"]
    volumes = {v["name"] for v in spec.get("volumes", [])}
    containers = spec.get("containers", []) + spec.get("initContainers", [])
    for container in containers:
        for mount in container.get("volumeMounts", []):
            if mount["name"] not in volumes:
                yield f"{doc['metadata']['name']}: mounts {mount['name']}, which no volume provides"


def env_value(doc, container_name, name):
    spec = doc["spec"]["template"]["spec"]
    for container in spec.get("containers", []):
        if container["name"] != container_name:
            continue
        for item in container.get("env", []):
            if item["name"] == name:
                return item.get("value")
    return None


def network_policy_declaration_matches(extra, docs):
    expected = (
        "true"
        if "worker.stdioEgress.networkPolicy.enforced=true" in extra
        else "false"
    )
    saw_serve = False
    for doc in docs:
        if doc and doc.get("kind") == "Deployment" and doc["metadata"]["name"].endswith("-serve"):
            saw_serve = True
            actual = env_value(doc, "serve", "FUSEONE_STDIO_EGRESS_NETWORK_POLICY_DECLARED")
            if actual != expected:
                return f"{' '.join(extra) or 'defaults'} — serve egress declaration = {actual!r}, want {expected!r}"
    if not saw_serve:
        return f"{' '.join(extra) or 'defaults'} — serve deployment was not rendered"
    return None


def env_names(doc, container_name):
    for container in doc["spec"]["template"]["spec"].get("containers", []):
        if container["name"] == container_name:
            return {item["name"] for item in container.get("env", [])}
    return set()


# Who holds the master key, and who has no business holding it.
#
# It seals every stored credential in the installation, and losing it means
# re-entering all of them. The processes that serve unseal credentials to reach
# connectors and model providers; the hook Jobs talk to Postgres and never
# unseal anything, so a short-lived pod holding the key would be the same
# mistake as mounting it a service account token.
KEY_HOLDERS = {
    "-serve": ("serve", True),
    "-worker": ("worker", True),
    "-migrate": ("migrate", False),
    "-reconcile-memory": ("reconcile-memory", False),
}


def master_key_reaches_only_what_unseals(extra, docs):
    seen = set()
    for doc in docs:
        if not doc or doc.get("kind") not in ("Deployment", "Job"):
            continue
        for suffix, (container, wants_key) in KEY_HOLDERS.items():
            if not doc["metadata"]["name"].endswith(suffix):
                continue
            seen.add(suffix)
            names = env_names(doc, container)
            where = f"{' '.join(extra) or 'defaults'} — {container}"
            if "DATABASE_URL" not in names:
                yield f"{where} has no DATABASE_URL"
            if wants_key and "FUSEONE_MASTER_KEY" not in names:
                yield f"{where} cannot unseal a credential: no FUSEONE_MASTER_KEY"
            if not wants_key and "FUSEONE_MASTER_KEY" in names:
                yield f"{where} holds FUSEONE_MASTER_KEY and never unseals anything"
    for suffix in KEY_HOLDERS.keys() - seen:
        yield f"{' '.join(extra) or 'defaults'} — nothing ending in {suffix} was rendered"


# A `- null` or a bare string in a containers list is valid YAML and a pod
# the API server refuses; every other check assumes mappings, so this one
# runs first and stops the combination.
def containers_are_mappings(extra, docs):
    for doc in docs:
        if not doc or doc.get("kind") not in CARRIES_PODS:
            continue
        spec = doc["spec"]["template"]["spec"]
        for container in spec.get("containers", []) + spec.get("initContainers", []):
            if not isinstance(container, dict):
                yield f"{' '.join(extra) or 'defaults'} — {doc['metadata']['name']}: container {container!r} is not a mapping"


def containers_of(docs, suffix):
    for doc in docs:
        if doc and doc.get("kind") == "Deployment" and doc["metadata"]["name"].endswith(suffix):
            return doc["spec"]["template"]["spec"]["containers"]
    return None


# The extra containers are the operator's, rendered exactly as given and only
# in the worker's pod. Unset, the worker has its one container and nothing
# stray: an empty default that rendered `- null` or `[]` would change every
# installation that never asked for a sidecar.
def extra_containers_are_verbatim(extra, docs):
    where = " ".join(extra) or "defaults"
    want = [SIDECAR] if HOOK[1] in extra else []
    worker = containers_of(docs, "-worker")
    if worker is None:
        yield f"{where} — worker deployment was not rendered"
    elif [c.get("name") for c in worker[:1]] != ["worker"] or worker[1:] != want:
        yield f"{where} — worker containers {worker[1:]!r} after its own, want {want!r}"
    serve = containers_of(docs, "-serve")
    if serve is not None and [c.get("name") for c in serve] != ["serve"]:
        yield f"{where} — serve containers {[c.get('name') for c in serve]}: the hook is the worker's"


def main():
    problems = []
    for extra in COMBINATIONS:
        rendered = subprocess.run(
            ["helm", "template", "check", CHART, "--set", "secret.existingSecret=x", *extra],
            capture_output=True, text=True,
        )
        if rendered.returncode != 0:
            problems.append(f"{' '.join(extra) or 'defaults'}: {rendered.stderr.strip().splitlines()[-1]}")
            continue
        docs = list(yaml.safe_load_all(rendered.stdout))
        malformed = list(containers_are_mappings(extra, docs))
        if malformed:
            problems.extend(malformed)
            continue
        problem =network_policy_declaration_matches(extra, docs)
        if problem:
            problems.append(problem)
        problems.extend(master_key_reaches_only_what_unseals(extra, docs))
        problems.extend(extra_containers_are_verbatim(extra, docs))
        for doc in docs:
            if doc and doc.get("kind") in CARRIES_PODS:
                problems.extend(f"{' '.join(extra) or 'defaults'} — {p}" for p in mounts_resolve(doc))

    for problem in problems:
        print(problem, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
