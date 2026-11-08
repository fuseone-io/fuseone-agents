import { useTranslation } from "react-i18next";
import { RevisedEditor } from "@/components/shared/revised-editor";
import { ChannelForm } from "@/features/channels/channel-form";
import { useChannelRevision } from "@/features/channels/api";
import type { components } from "@/lib/api/schema.gen";

/**
 * The connection sheet, seeded from the connection as it is stored now. Its
 * revision also names the conversations and identities attached to it.
 */
export function ChannelEditor({
  channel,
  kinds,
  onClose,
}: {
  channel: components["schemas"]["Channel"] | null;
  kinds: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const read = useChannelRevision(channel?.name);

  // A connection being added has nothing to read.
  if (!channel) return <ChannelForm channel={null} kinds={kinds} onClose={onClose} />;
  return (
    <RevisedEditor
      read={read}
      title={t("channels.editChannel")}
      description={t("channels.explains")}
      onClose={onClose}
    >
      {(stored, revision) => (
        // A new revision is a new form: reloading replaces the draft with it.
        <ChannelForm
          key={revision.etag}
          channel={stored}
          kinds={kinds}
          onClose={onClose}
          revision={revision}
        />
      )}
    </RevisedEditor>
  );
}
