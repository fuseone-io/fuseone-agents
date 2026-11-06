import { useTranslation } from "react-i18next";
import {
  PropertiesSheet,
  PropertiesSheetBody,
} from "@/components/shared/properties-sheet";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { useConnectorInstance } from "@/features/integrations/api";
import type { ConnectorEditorProps } from "@/features/integrations/connectors/connector-editor-registry";
import { EvidenceInstanceForm } from "@/features/integrations/connectors/evidence-instance-form";

/**
 * The listing carries only a summary of a declaration, so editing reads the
 * detail first. Opening the form on the summary would save a source with no
 * queries.
 */
export function EvidenceInstanceEditor({
  instance,
  onClose,
  onSave,
}: ConnectorEditorProps) {
  const { t } = useTranslation();
  const detail = useConnectorInstance(instance ?? undefined);
  if (!instance) {
    return (
      <EvidenceInstanceForm instance={null} onClose={onClose} onSave={onSave} />
    );
  }
  if (!detail.isLoading && !detail.isFetching && detail.data) {
    return (
      <EvidenceInstanceForm
        instance={detail.data}
        onClose={onClose}
        onSave={onSave}
      />
    );
  }
  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("connectors.editInstance")}
      description={t("connectors.evidenceSheetHint")}
    >
      <PropertiesSheetBody>
        {detail.error ? (
          <ErrorState
            error={detail.error}
            onRetry={() => void detail.refetch()}
          />
        ) : (
          <LoadingRows rows={5} />
        )}
      </PropertiesSheetBody>
    </PropertiesSheet>
  );
}
