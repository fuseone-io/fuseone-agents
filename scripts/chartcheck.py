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

import subprocess
import sys

import yaml

CHART = "deploy/helm/fuseone-agents"
COMBINATIONS = [
    [],
    ["--set", "worker.specs.configMap=specs"],
    ["--set", "ingress.enabled=true", "--set", "ingress.host=a.example.com"],
    ["--set", "gateway.enabled=true", "--set", "gateway.parentRefs[0].name=g"],
    ["--set", "networkPolicy.enabled=true"],
    ["--set", "worker.stdioEgress.networkPolicy.enforced=true"],
    ["--set", "networkPolicy.enabled=true", "--set", "worker.stdioEgress.networkPolicy.enforced=true"],
    ["--set", "topologySpread.enabled=false"],
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
        problem = network_policy_declaration_matches(extra, docs)
        if problem:
            problems.append(problem)
        for doc in docs:
            if doc and doc.get("kind") in CARRIES_PODS:
                problems.extend(f"{' '.join(extra) or 'defaults'} — {p}" for p in mounts_resolve(doc))

    for problem in problems:
        print(problem, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
