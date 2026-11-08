import { useTranslation } from "react-i18next";
import { answering } from "@/features/integrations/connectors/connector-instance-model";
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
  if (detail.isLoading || detail.isFetching) {
    return (
      <PropertiesSheet
        open
        onOpenChange={(open) => !open && onClose()}
        title={t("connectors.editSQLInstance")}
        description={t("connectors.loadingSQLInstance")}
      >
        <PropertiesSheetBody>
          <LoadingRows rows={5} />
        </PropertiesSheetBody>
      </PropertiesSheet>
    );
  }
  if (detail.error || !detail.data) {
    return (
      <PropertiesSheet
        open
        onOpenChange={(open) => !open && onClose()}
        title={t("connectors.editSQLInstance")}
        description={t("connectors.sqlInstanceSheetHint")}
      >
        <PropertiesSheetBody>
          <ErrorState
            error={detail.error ?? new Error("SQL instance detail is missing")}
            onRetry={() => void detail.refetch()}
          />
        </PropertiesSheetBody>
      </PropertiesSheet>
    );
  }
  return (
    <SQLInstanceForm
      // A new revision is a new form: reloading replaces the draft with it.
      key={detail.data.etag}
      instance={detail.data.body}
      instances={instances}
      onClose={onClose}
      onSave={answering(onSave, detail.data.etag)}
      revision={{
        etag: detail.data.etag,
        reload: () => void detail.refetch(),
        reloading: detail.isFetching,
      }}
    />
  );
}
