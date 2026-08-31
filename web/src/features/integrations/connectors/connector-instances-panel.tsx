import { Database, Plus, ShieldCheck, Workflow } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Panel } from "@/components/shared/panel";
import type { ConnectorInstance } from "@/features/integrations/api";
import { ConnectorInstanceCard } from "@/features/integrations/connectors/connector-instance-card";

type ConnectorInstancesView = {
  instances: ConnectorInstance[];
  isLoading: boolean;
  error: unknown;
  canConfigure: boolean;
};

type ConnectorInstancesActions = {
  retry: () => void;
  create: (connector: "vault" | "sql") => void;
  edit: (instance: ConnectorInstance) => void;
  remove: (instance: ConnectorInstance) => void;
};

export function ConnectorInstancesPanel({
  view,
  actions,
}: {
  view: ConnectorInstancesView;
  actions: ConnectorInstancesActions;
}) {
  const { t } = useTranslation();
  return (
    <Panel
      title={t("connectors.instances")}
      action={
        view.canConfigure ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" size="sm">
                <Plus className="size-4" aria-hidden />
                {t("connectors.newInstance")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => actions.create("vault")}>
                <ShieldCheck aria-hidden />
                {t("connectors.newVault")}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => actions.create("sql")}>
                <Database aria-hidden />
                {t("connectors.newSQL")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : undefined
      }
    >
      {view.isLoading ? (
        <LoadingRows rows={2} />
      ) : view.error ? (
        <ErrorState error={view.error} onRetry={actions.retry} />
      ) : view.instances.length === 0 ? (
        <EmptyState
          icon={<Workflow className="size-6" />}
          title={t("connectors.noInstances")}
          hint={t("connectors.noInstancesHint")}
        />
      ) : (
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(min(100%,320px),1fr))]">
          {view.instances.map((instance) => (
            <ConnectorInstanceCard
              key={`${instance.scopeKind}:${instance.company ?? ""}:${instance.area ?? ""}:${instance.name}`}
              instance={instance}
              onEdit={
                view.canConfigure &&
                (instance.connector === "vault" || instance.connector === "sql")
                  ? () => actions.edit(instance)
                  : undefined
              }
              onDelete={() => actions.remove(instance)}
            />
          ))}
        </div>
      )}
    </Panel>
  );
}
