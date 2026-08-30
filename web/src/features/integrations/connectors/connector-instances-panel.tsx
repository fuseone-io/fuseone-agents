import { Plus, Workflow } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Panel } from "@/components/shared/panel";
import type { ConnectorInstance } from "@/features/integrations/api";
import { ConnectorInstanceCard } from "@/features/integrations/connectors/connector-instance-card";

type ConnectorInstancesView = {
  instances: ConnectorInstance[];
  isLoading: boolean;
  error: unknown;
};

type ConnectorInstancesActions = {
  retry: () => void;
  createVault: () => void;
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
        <Button type="button" size="sm" onClick={actions.createVault}>
          <Plus className="size-4" aria-hidden />
          {t("connectors.newVault")}
        </Button>
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
              onEdit={instance.connector === "vault" ? () => actions.edit(instance) : undefined}
              onDelete={() => actions.remove(instance)}
            />
          ))}
        </div>
      )}
    </Panel>
  );
}
