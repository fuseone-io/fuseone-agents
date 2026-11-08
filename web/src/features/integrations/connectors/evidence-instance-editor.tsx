import { useTranslation } from "react-i18next";
import { answering } from "@/features/integrations/connectors/connector-instance-model";
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
  const settled = !detail.isLoading && !detail.isFetching;
  // The error first. A failed refetch keeps the last answer in `data`, and
  // opening the form on it would let a save overwrite a newer declaration.
  if (settled && !detail.error && detail.data) {
    return (
      <EvidenceInstanceForm
        // A new revision is a new form: reloading replaces the draft with it.
        key={detail.data.etag}
        instance={detail.data.body}
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
  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("connectors.editInstance")}
      description={t("connectors.evidenceSheetHint")}
    >
      <PropertiesSheetBody>
        {settled ? (
          <ErrorState
            error={
              detail.error ?? new Error("evidence instance detail is missing")
            }
            onRetry={() => void detail.refetch()}
          />
        ) : (
          <LoadingRows rows={5} />
        )}
      </PropertiesSheetBody>
    </PropertiesSheet>
  );
}
