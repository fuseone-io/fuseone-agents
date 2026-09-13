import type {
  ConnectorInstance,
  ConnectorScopeKind,
} from "@/features/integrations/api";

export type ConnectorTarget = {
  scopeKind: ConnectorScopeKind;
  company: string;
  area: string;
};

export type VaultChoice = {
  name: string;
  label: string;
  ambiguous: boolean;
};

export function vaultChoices(
  instances: ConnectorInstance[],
  target: ConnectorTarget,
): VaultChoice[] {
  const grouped = new Map<string, ConnectorInstance[]>();
  for (const instance of instances) {
    if (!covers(instance, target)) continue;
    grouped.set(instance.name, [...(grouped.get(instance.name) ?? []), instance]);
  }
  return [...grouped.entries()]
    .flatMap(([name, matches]) => {
      if (!matches.some(usableVault)) return [];
      return [{
        name,
        label: matches.length === 1 ? `${name} · ${scopeLabel(matches[0]!)}` : name,
        ambiguous: matches.length !== 1,
      }];
    })
    .sort((left, right) => left.name.localeCompare(right.name));
}

export function vaultsNamed(
  instances: ConnectorInstance[],
  target: ConnectorTarget,
  name: string,
): ConnectorInstance[] {
  return instances.filter((instance) => instance.name === name && covers(instance, target));
}

export function usableVault(instance: ConnectorInstance): boolean {
  return instance.connector === "vault" && instance.enabled && instance.hasToken &&
    instance.vault?.address.startsWith("https://") === true;
}

export function vaultOwnsPath(instance: ConnectorInstance, raw: string): boolean {
  const wanted = cleanVaultPath(raw);
  if (!wanted) return false;
  return (instance.vault?.allowedPathPrefixes ?? []).some((candidate) => {
    const prefix = cleanVaultPath(candidate);
    return prefix !== "" && (wanted === prefix || wanted.startsWith(`${prefix}/`));
  });
}

function covers(source: ConnectorInstance, target: ConnectorTarget): boolean {
  if (source.scopeKind === "installation") return true;
  if (target.scopeKind === "installation") return false;
  if (source.company !== target.company) return false;
  if (source.scopeKind === "company") return true;
  return target.scopeKind === "area" && source.area === target.area;
}

function scopeLabel(instance: ConnectorInstance): string {
  if (instance.scopeKind === "installation") return "installation";
  if (instance.scopeKind === "company") return instance.company ?? "-";
  return `${instance.company ?? "-"}/${instance.area ?? "-"}`;
}

function cleanVaultPath(raw: string): string {
  const parts = raw.trim().split("/").filter(Boolean);
  if (parts.length === 0 || parts.some((part) => part === "." || part === "..")) {
    return "";
  }
  return parts.join("/");
}
