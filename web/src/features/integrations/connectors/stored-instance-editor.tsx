import type { ComponentType } from "react";
import { useTranslation } from "react-i18next";
import { RevisedEditor } from "@/components/shared/revised-editor";
import { useConnectorInstance } from "@/features/integrations/api";
import type { ConnectorEditorProps } from "@/features/integrations/connectors/connector-editor-registry";
import {
  answering,
  listedShape,
} from "@/features/integrations/connectors/connector-instance-model";

/**
 * An instance form seeded from the instance as it is stored now.
 *
 * For the forms that used to start from the listing's row: that row is as old
 * as the list, and a save over it would answer a revision nobody saw. A new
 * instance has nothing to read and goes straight to the form.
 */
export function storedFirst(
  Form: ComponentType<ConnectorEditorProps>,
): ComponentType<ConnectorEditorProps> {
  return function StoredInstanceEditor(props: ConnectorEditorProps) {
    const { t } = useTranslation();
    const read = useConnectorInstance(props.instance ?? undefined);
    if (!props.instance) return <Form {...props} />;
    return (
      <RevisedEditor
        read={read}
        title={t("connectors.editInstance")}
        description={t("connectors.instanceSheetHint")}
        onClose={props.onClose}
      >
        {(stored, revision) => (
          // A new revision is a new form: reloading replaces the draft with it.
          <Form
            key={revision.etag}
            {...props}
            instance={listedShape(stored)}
            onSave={answering(props.onSave, revision.etag)}
            revision={revision}
          />
        )}
      </RevisedEditor>
    );
  };
}
