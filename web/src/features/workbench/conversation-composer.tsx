import { useTranslation } from "react-i18next";
import { SendHorizontal } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { Composer } from "@/features/workbench/use-composer";

/** At most what one turn carries (NT-018 phase 3, 16 KiB). */
export const MAX_MESSAGE_CHARS = 16_384;

/** What the person writes to the assistant. Closed, it reads and sends nothing. */
export function ConversationComposer({ composer, open }: { composer: Composer; open: boolean }) {
  const { t } = useTranslation();
  const blocked = !open || composer.sending;

  return (
    <div className="flex flex-col gap-2 border-t border-border p-4">
      <div className="flex items-center justify-between gap-2">
        <Label htmlFor="conversation-message">{t("workbench.conversation.message")}</Label>
        {composer.pasted && <Badge variant="outline">{t("workbench.conversation.pasted")}</Badge>}
      </div>
      <Textarea
        id="conversation-message"
        value={composer.text}
        maxLength={MAX_MESSAGE_CHARS}
        disabled={blocked}
        placeholder={t("workbench.conversation.placeholder")}
        onChange={(e) => composer.write(e.target.value)}
        onPaste={composer.paste}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) composer.send();
        }}
      />
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">{t("workbench.conversation.untrusted")}</span>
        <Button size="sm" onClick={composer.send} disabled={blocked || composer.text.trim() === ""}>
          <SendHorizontal className="size-4" aria-hidden />
          {t("workbench.conversation.send")}
        </Button>
      </div>
    </div>
  );
}
