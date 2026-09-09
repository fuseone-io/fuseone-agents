import { useTranslation } from "react-i18next";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { Button } from "@/components/ui/button";

/**
 * A conversation this console cannot draw.
 *
 * Its mode was written by a newer version, and the platform keeps it as it is:
 * a mode nothing here can name starts nothing, so the conversation is safe
 * where it stands. Offering the form would not be. Every field would be filled
 * with this console's idea of the nearest value, and saving would write that
 * back — which is how a room that started nothing became one anybody could
 * start runs from by typing in it.
 */
export function ConversationUnknownMode({
  mode,
  onClose,
}: {
  mode: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("channels.editConversation")}
      description={t("channels.conversationExplains")}
    >
      <PropertiesSheetBody>
        <p className="text-sm text-muted-foreground">
          {t("channels.unknownMode", { mode })}
        </p>
      </PropertiesSheetBody>
      <PropertiesSheetFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.close")}
        </Button>
      </PropertiesSheetFooter>
    </PropertiesSheet>
  );
}
