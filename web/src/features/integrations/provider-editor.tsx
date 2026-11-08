import { useTranslation } from "react-i18next";
import { RevisedEditor } from "@/components/shared/revised-editor";
import { ProviderForm } from "@/features/integrations/provider-form";
import {
  useProviderRevision,
  type ModelProvider,
} from "@/features/integrations/api";

/** The provider sheet, seeded from the provider as it is stored now. */
export function ProviderEditor({
  provider,
  onClose,
}: {
  provider: ModelProvider | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const read = useProviderRevision(provider?.name);

  // A provider being added has nothing to read.
  if (!provider) return <ProviderForm provider={null} onClose={onClose} />;
  return (
    <RevisedEditor
      read={read}
      title={t("integrations.editProvider")}
      description={t("integrations.credentialSealed")}
      onClose={onClose}
    >
      {(stored, revision) => (
        // A new revision is a new form: reloading replaces the draft with it.
        <ProviderForm key={revision.etag} provider={stored} onClose={onClose} revision={revision} />
      )}
    </RevisedEditor>
  );
}
