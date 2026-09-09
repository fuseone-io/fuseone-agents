import { useTranslation } from "react-i18next";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { Button } from "@/components/ui/button";

/**
 * Configuration this console cannot draw.
 *
 * Something in it — a conversation's start mode, a connection's delivery mode —
 * was written by a newer version, and the platform keeps it as it is: a value
 * nothing here can name starts nothing and opens nothing, so what is stored is
 * safe where it stands. Offering the form would not be. Every field would be
 * filled with this console's idea of the nearest value, and saving would write
 * that back — which is how a room that started nothing became one anybody could
 * start runs from by typing in it.
 */
export function UnknownConfiguration({
  title,
  message,
  onClose,
}: {
  title: string;
  message: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={title}
      description={t("channels.unknownConfiguration")}
    >
      <PropertiesSheetBody>
        <p className="text-sm text-muted-foreground">{message}</p>
      </PropertiesSheetBody>
      <PropertiesSheetFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.close")}
        </Button>
      </PropertiesSheetFooter>
    </PropertiesSheet>
  );
}
