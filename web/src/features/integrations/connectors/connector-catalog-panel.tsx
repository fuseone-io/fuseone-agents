import { Plus, ShieldCheck, Workflow } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import type {
  ConnectorInstance,
  GovernedConnector,
} from "@/features/integrations/api";
import { ConnectorInstanceForm } from "@/features/integrations/connectors/connector-instance-form";
import { ConnectorInstancesPanel } from "@/features/integrations/connectors/connector-instances-panel";
import type { ConnectorInstanceSaver } from "@/features/integrations/connectors/connector-instance-model";
import { SQLInstanceEditor } from "@/features/integrations/connectors/sql-instance-editor";
import { useVisibleItems } from "@/hooks/use-visible-items";
import { cn } from "@/lib/utils";

export function ConnectorCatalogPanel({
  data,
  actions,
}: {
  data: ConnectorPanelData;
  actions: ConnectorPanelActions;
}) {
  const [editing, setEditing] = useState<{
    connector: "vault" | "sql";
    instance: ConnectorInstance | null;
  } | null>(null);
  const page = useVisibleItems(data.connectors, 8);

  const close = () => setEditing(null);

  return (
    <section className="flex flex-col gap-3">
      <ConnectorIntro />
      <ConnectorInstancesPanel
        view={{
          instances: data.instances,
          isLoading: data.instancesLoading,
          error: data.instancesError,
        }}
        actions={{
          retry: actions.retryInstances,
          create: (connector) => setEditing({ connector, instance: null }),
          edit: (instance) => {
            if (configurableConnector(instance.connector)) {
              setEditing({ connector: instance.connector, instance });
            }
          },
          remove: actions.deleteInstance,
        }}
      />
      {data.catalogLoading ? (
        <LoadingRows rows={4} />
      ) : data.catalogError ? (
        <ErrorState error={data.catalogError} onRetry={actions.retryCatalog} />
      ) : (
        <ConnectorCatalogBody
          connectors={data.connectors}
          page={page}
          onConfigure={(connector) => {
            if (
              (connector.id === "vault" || connector.id === "sql") &&
              connector.maturity === "runtime"
            ) {
              setEditing({ connector: connector.id, instance: null });
            }
          }}
        />
      )}
      {editing?.connector === "vault" && (
        <ConnectorInstanceForm
          instance={editing.instance}
          onClose={close}
          onSave={actions.saveInstance}
        />
      )}
      {editing?.connector === "sql" && (
        <SQLInstanceEditor
          instance={editing.instance}
          instances={data.instances}
          onClose={close}
          onSave={actions.saveInstance}
        />
      )}
    </section>
  );
}

function configurableConnector(
  connector: string,
): connector is "vault" | "sql" {
  return connector === "vault" || connector === "sql";
}

export type ConnectorPanelData = {
  connectors: GovernedConnector[];
  instances: ConnectorInstance[];
  catalogLoading: boolean;
  instancesLoading: boolean;
  catalogError: unknown;
  instancesError: unknown;
};

export type ConnectorPanelActions = {
  retryCatalog: () => void;
  retryInstances: () => void;
  saveInstance: ConnectorInstanceSaver;
  deleteInstance: (instance: ConnectorInstance) => void;
};

function ConnectorIntro() {
  const { t } = useTranslation();
  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="flex items-start gap-3">
        <div className="grid size-9 shrink-0 place-items-center rounded-md border bg-muted">
          <ShieldCheck className="size-4 text-primary" aria-hidden />
        </div>
        <div className="min-w-0">
          <h2 className="text-sm font-medium">{t("connectors.title")}</h2>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            {t("connectors.subtitle")}
          </p>
        </div>
      </div>
    </div>
  );
}

function ConnectorCatalogBody({
  connectors,
  page,
  onConfigure,
}: {
  connectors: GovernedConnector[];
  page: ReturnType<typeof useVisibleItems<GovernedConnector>>;
  onConfigure?: (connector: GovernedConnector) => void;
}) {
  const { t } = useTranslation();
  if (connectors.length === 0) {
    return (
      <EmptyState
        icon={<Workflow className="size-6" />}
        title={t("connectors.emptyTitle")}
        hint={t("connectors.emptyHint")}
      />
    );
  }
  return (
    <>
      <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(min(100%,308px),1fr))]">
        {page.visible.map((connector) => (
          <ConnectorCard
            key={connector.id}
            connector={connector}
            onConfigure={
              onConfigure &&
              (connector.id === "vault" || connector.id === "sql")
                ? () => onConfigure(connector)
                : undefined
            }
          />
        ))}
      </div>
      <LoadMore {...page} isLoading={false} onLoad={page.loadMore} />
    </>
  );
}

function ConnectorCard({
  connector,
  onConfigure,
}: {
  connector: GovernedConnector;
  onConfigure?: () => void;
}) {
  return (
    <article className="flex min-h-[22rem] flex-col overflow-hidden rounded-lg border bg-card">
      <ConnectorCardHeader connector={connector} onConfigure={onConfigure} />

      <div className="flex flex-1 flex-col gap-4 p-4">
        <ConnectorOperations connector={connector} />
        <ConnectorGuarantees connector={connector} />
      </div>
    </article>
  );
}

function ConnectorCardHeader({
  connector,
  onConfigure,
}: {
  connector: GovernedConnector;
  onConfigure?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-start gap-3 border-b p-4">
      <div className="grid size-9 shrink-0 place-items-center rounded-md border bg-muted">
        <Workflow className="size-4 text-muted-foreground" aria-hidden />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <h3 className="truncate text-sm font-medium">{connector.name}</h3>
          <Badge variant="outline" className="ml-auto shrink-0">
            {t(`connectors.maturity.${connector.maturity}`)}
          </Badge>
        </div>
        <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
          {connector.summary}
        </p>
        {connector.maturity === "runtime" && onConfigure && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="mt-3"
            onClick={onConfigure}
          >
            <Plus className="size-3.5" aria-hidden />
            {t("connectors.configure")}
          </Button>
        )}
      </div>
    </div>
  );
}

function ConnectorOperations({ connector }: { connector: GovernedConnector }) {
  const { t } = useTranslation();
  return (
    <div>
      <div className="mb-2 flex items-center gap-2 text-xs font-medium uppercase text-muted-foreground">
        {t("connectors.operations")}
        <span className="font-mono text-2xs">{connector.operations.length}</span>
      </div>
      <div className="space-y-2">
        {connector.operations.slice(0, 3).map((operation) => (
          <OperationRow key={operation.id} operation={operation} />
        ))}
      </div>
    </div>
  );
}

function ConnectorGuarantees({ connector }: { connector: GovernedConnector }) {
  const { t } = useTranslation();
  return (
    <div className="mt-auto border-t pt-3">
      <p className="text-xs font-medium uppercase text-muted-foreground">
        {t("connectors.guarantees")}
      </p>
      <ul className="mt-2 space-y-1.5 text-sm text-muted-foreground">
        {connector.guarantees.slice(0, 2).map((guarantee) => (
          <li key={guarantee} className="flex gap-2">
            <span aria-hidden className="mt-2 size-1 rounded-full bg-primary" />
            <span>{guarantee}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function OperationRow({
  operation,
}: {
  operation: GovernedConnector["operations"][number];
}) {
  const { t } = useTranslation();
  return (
    <div className="rounded-md border bg-background p-2.5">
      <div className="flex items-center gap-2">
        <p className="truncate text-sm font-medium">{operation.name}</p>
        <Badge variant="secondary" className="ml-auto shrink-0 text-2xs">
          {t(`connectors.approval.${operation.approval}`)}
        </Badge>
      </div>
      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
        {operation.summary}
      </p>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {operation.effects.map((effect) => (
          <Badge key={effect} variant="outline" className={effectClass(effect)}>
            {t(`connectors.effect.${effect}`)}
          </Badge>
        ))}
        <Badge variant="outline" className="text-2xs">
          {t(`connectors.secretHandling.${operation.secretHandling}`)}
        </Badge>
      </div>
    </div>
  );
}

function effectClass(effect: GovernedConnector["operations"][number]["effects"][number]) {
  return cn(
    "text-2xs",
    (effect === "write" || effect === "destructive") &&
      "border-primary/50 text-primary",
  );
}
