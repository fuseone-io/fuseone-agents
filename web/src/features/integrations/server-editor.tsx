import { useTranslation } from "react-i18next";
import { RevisedEditor } from "@/components/shared/revised-editor";
import { ServerForm } from "@/features/integrations/server-form";
import {
  useMCPServerRevision,
  type MCPServer,
} from "@/features/integrations/api";

/** The server sheet, seeded from the server as it is stored now. */
export function ServerEditor({
  server,
  onClose,
}: {
  server: MCPServer | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const read = useMCPServerRevision(server?.name);

  // A server being connected has nothing to read.
  if (!server) return <ServerForm server={null} onClose={onClose} />;
  return (
    <RevisedEditor
      read={read}
      title={t("integrations.editServer")}
      description={t("integrations.mcpExplains")}
      onClose={onClose}
    >
      {(stored, revision) => (
        // A new revision is a new form: reloading replaces the draft with it.
        <ServerForm key={revision.etag} server={stored} onClose={onClose} revision={revision} />
      )}
    </RevisedEditor>
  );
}
