import { Pencil } from "lucide-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RemoveButton } from "@/components/shared/remove-button";
import type { ConnectorInstance } from "@/features/integrations/api";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

export function ConnectorInstanceCard({
  instance,
  onEdit,
  onDelete,
}: {
  instance: ConnectorInstance;
  onEdit?: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <article className="rounded-lg border bg-background p-3">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <h3 className="truncate font-mono text-sm font-medium">
              {instance.name}
            </h3>
            <Badge variant={instance.enabled ? "secondary" : "outline"}>
              {instance.enabled ? t("connectors.enabled") : t("common.disabled")}
            </Badge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("connectors.instanceSummary", {
              connector: instance.connector,
              scope: scopeLabel(instance, t),
            })}
          </p>
        </div>
        {onEdit && (
          <Button type="button" variant="ghost" size="icon" onClick={onEdit}>
            <Pencil className="size-4" aria-hidden />
            <span className="sr-only">{t("common.edit")}</span>
          </Button>
        )}
        <RemoveButton
          title={t("connectors.removeInstance")}
          description={t("connectors.removeInstanceHint", {
            name: instance.name,
          })}
          onConfirm={onDelete}
        />
      </div>
      <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
        {instance.connector === "sql" ? (
          <SQLFacts instance={instance} t={t} />
        ) : (
          <VaultFacts instance={instance} t={t} />
        )}
      </div>
    </article>
  );
}

function VaultFacts({ instance, t }: { instance: ConnectorInstance; t: TFunction }) {
  return (
    <>
      <Fact label={t("connectors.token")} value={tokenState(instance, t)} />
      <Fact label={t("connectors.updatedAt")} value={updated(instance, t)} />
      <Fact label={t("connectors.vaultMount")} value={instance.vault?.mount ?? "-"} mono />
      <Fact
        label={t("connectors.allowedPathPrefixes")}
        value={(instance.vault?.allowedPathPrefixes ?? []).join(", ") || "-"}
        mono
      />
    </>
  );
}

function SQLFacts({ instance, t }: { instance: ConnectorInstance; t: TFunction }) {
  const sql = instance.sql;
  const source = sql?.credentialSource;
  return (
    <>
      <Fact label={t("connectors.sqlDriver")} value={sql?.driver ?? "-"} mono />
      <Fact label={t("connectors.updatedAt")} value={updated(instance, t)} />
      <Fact
        label={t("connectors.sqlTarget")}
        value={sql ? `${sql.host}:${sql.port}/${sql.database}` : "-"}
        mono
      />
      <Fact
        label={t("connectors.sqlCredentialSource")}
        value={source ? `${source.vaultInstance}/${source.role}` : "-"}
        mono
      />
      <Fact label={t("connectors.sqlTemplates")} value={String(sql?.templates.length ?? 0)} />
    </>
  );
}

function Fact({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <p className="text-2xs uppercase text-muted-foreground">{label}</p>
      <p className={cn("truncate", mono && "font-mono")}>{value}</p>
    </div>
  );
}

function scopeLabel(instance: ConnectorInstance, t: TFunction) {
  if (instance.scopeKind === "installation") {
    return t("connectors.scope.installation");
  }
  if (instance.scopeKind === "company") return instance.company ?? "-";
  return `${instance.company ?? "-"}/${instance.area ?? "-"}`;
}

function tokenState(instance: ConnectorInstance, t: TFunction) {
  return instance.hasToken
    ? t("connectors.tokenStored")
    : t("connectors.tokenMissing");
}

function updated(instance: ConnectorInstance, t: TFunction) {
  return instance.updatedAt ? formatRelative(instance.updatedAt) : t("connectors.never");
}
