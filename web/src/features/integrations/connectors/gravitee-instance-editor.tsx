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
import { GraviteeInstanceForm } from "@/features/integrations/connectors/gravitee-instance-form";
import type { ConnectorInstanceSaver } from "@/features/integrations/connectors/connector-instance-model";

export function GraviteeInstanceEditor({
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
      <GraviteeInstanceForm
        instance={null}
        instances={instances}
        onClose={onClose}
        onSave={onSave}
      />
    );
  }
  return (
    <ExistingGraviteeInstanceEditor
      instance={instance}
      instances={instances}
      onClose={onClose}
      onSave={onSave}
    />
  );
}

function ExistingGraviteeInstanceEditor({
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
      <EditorShell
        title={t("connectors.editGraviteeInstance")}
        description={t("connectors.loadingGraviteeInstance")}
        onClose={onClose}
      >
        <LoadingRows rows={5} />
      </EditorShell>
    );
  }
  if (detail.error || !detail.data) {
    return (
      <EditorShell
        title={t("connectors.editGraviteeInstance")}
        description={t("connectors.graviteeInstanceSheetHint")}
        onClose={onClose}
      >
        <ErrorState
          error={
            detail.error ?? new Error("Gravitee instance detail is missing")
          }
          onRetry={() => void detail.refetch()}
        />
      </EditorShell>
    );
  }
  return (
    <GraviteeInstanceForm
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

function EditorShell({
  title,
  description,
  onClose,
  children,
}: {
  title: string;
  description: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={title}
      description={description}
    >
      <PropertiesSheetBody>{children}</PropertiesSheetBody>
    </PropertiesSheet>
  );
}
