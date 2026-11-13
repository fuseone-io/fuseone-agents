import type { TFunction } from "i18next";

type Stored = {
  scopeKind?: "installation" | "company" | "area";
  company?: string;
  area?: string;
};

/** Where a row is stored, as the console names it: acme/cx, acme, or the installation. */
export function scopeLabel(row: Stored, t: TFunction) {
  if (!row.scopeKind || row.scopeKind === "installation") {
    return t("connectors.scope.installation");
  }
  if (row.scopeKind === "company") return row.company ?? "-";
  return `${row.company ?? "-"}/${row.area ?? "-"}`;
}

// Listed and not editable is inherited: every listing an area's configurer
// reads holds only its own rows and those of the scopes above it.
export function inheritedFrom(row: Stored) {
  return !row.scopeKind || row.scopeKind === "installation"
    ? "connectors.inheritedFromPlatform"
    : "connectors.inheritedFromCompany";
}
