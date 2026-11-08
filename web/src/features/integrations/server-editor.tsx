import { useTranslation } from "react-i18next";
import {
  PropertiesSheet,
  PropertiesSheetBody,
} from "@/components/shared/properties-sheet";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { ServerForm } from "@/features/integrations/server-form";
import {
  useMCPServerRevision,
  type MCPServer,
} from "@/features/integrations/api";

/**
 * The server sheet, seeded from the server as it is stored now.
 *
 * Read by name as it opens, never from the row it was opened from: that row is
 * as old as the list, and a save over it would answer a revision the person
 * never saw. A server being connected has nothing to read.
 */
export function ServerEditor({
  server,
  onClose,
}: {
  server: MCPServer | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const read = useMCPServerRevision(server?.name);

  if (!server) return <ServerForm server={null} onClose={onClose} />;
  if (!read.data) {
    return (
      <PropertiesSheet
        open
        onOpenChange={(open) => !open && onClose()}
        title={t("integrations.editServer")}
        description={t("integrations.mcpExplains")}
      >
        <PropertiesSheetBody>
          {read.error ? (
            <ErrorState
              error={read.error}
              onRetry={() => void read.refetch()}
            />
          ) : (
            <LoadingRows rows={4} />
          )}
        </PropertiesSheetBody>
      </PropertiesSheet>
    );
  }
  return (
    <ServerForm
      // A new revision is a new form: reloading replaces the draft with it.
      key={read.data.etag}
      server={read.data.body}
      onClose={onClose}
      revision={{
        etag: read.data.etag,
        reload: () => void read.refetch(),
        reloading: read.isFetching,
      }}
    />
  );
}
