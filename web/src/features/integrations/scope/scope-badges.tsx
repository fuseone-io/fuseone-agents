import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { inheritedFrom, scopeLabel } from "@/features/integrations/scope/scope-label";

type ScopedRow = {
  scopeKind?: "installation" | "company" | "area";
  company?: string;
  area?: string;
  editable?: boolean;
};

/**
 * Where an integration is stored, and whether the caller only inherits it
 * (NT-019): a server or a model provider alike. A row that predates scopes
 * says nothing, rather than a guess.
 */
export function ScopeBadges({ row }: { row: ScopedRow }) {
  const { t } = useTranslation();
  if (!row.scopeKind) return null;
  return (
    <>
      <Badge variant="secondary" className="font-mono">
        {scopeLabel(row, t)}
      </Badge>
      {row.editable === false && <Badge variant="outline">{t(inheritedFrom(row))}</Badge>}
    </>
  );
}
