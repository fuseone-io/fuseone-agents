import { useTranslation } from "react-i18next";
import {
  PropertiesSheet,
  PropertiesSheetBody,
} from "@/components/shared/properties-sheet";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import {
  useConnectorInstance,
  type ConnectorInstance,
} from "@/features/integrations/api";
import type { ConnectorInstanceSaver } from "@/features/integrations/connectors/connector-instance-model";
import { SQLInstanceForm } from "@/features/integrations/connectors/sql-instance-form";

export function SQLInstanceEditor({
  instance,
  instances,
  onClose,
  onSave,
}: {
  instance: ConnectorInstance | null;
  instances: ConnectorInstance[];
  onClose: () => void;
  onSave: ConnectorInstanceSaver;
}) {
  if (!instance) {
    return (
      <SQLInstanceForm
        instance={null}
        instances={instances}
        onClose={onClose}
        onSave={onSave}
      />
    );
  }
  return (
    <ExistingSQLInstanceEditor
      instance={instance}
      instances={instances}
      onClose={onClose}
      onSave={onSave}
    />
  );
}

function ExistingSQLInstanceEditor({
  instance,
  instances,
  onClose,
  onSave,
}: {
  instance: ConnectorInstance;
  instances: ConnectorInstance[];
  onClose: () => void;
  onSave: ConnectorInstanceSaver;
}) {
  const { t } = useTranslation();
  const detail = useConnectorInstance(instance);
  if (detail.isLoading) {
    return (
      <PropertiesSheet
        open
        onOpenChange={(open) => !open && onClose()}
        title={t("connectors.editSQLInstance")}
        description={t("connectors.loadingSQLInstance")}
      >
        <PropertiesSheetBody><LoadingRows rows={5} /></PropertiesSheetBody>
      </PropertiesSheet>
    );
  }
  if (detail.error) {
    return (
      <PropertiesSheet
        open
        onOpenChange={(open) => !open && onClose()}
        title={t("connectors.editSQLInstance")}
        description={t("connectors.sqlInstanceSheetHint")}
      >
        <PropertiesSheetBody>
          <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />
        </PropertiesSheetBody>
      </PropertiesSheet>
    );
  }
  return (
    <SQLInstanceForm
      instance={detail.data ?? null}
      instances={instances}
      onClose={onClose}
      onSave={onSave}
    />
  );
}
