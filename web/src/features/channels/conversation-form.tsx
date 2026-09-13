import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  PropertiesSheet,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { useSaveConversation } from "@/features/channels/api";
import { ConversationFormFields } from "@/features/channels/conversation-form-fields";
import {
  conversationSchema,
  knownMode,
  type ConversationValues,
} from "@/features/channels/conversation-form-model";
import {
  conversationDefaults,
  conversationInput,
  type Conversation,
  unsupportedConversation,
} from "@/features/channels/conversation-form-state";
import { INSTALLATION_SCOPE } from "@/features/channels/conversation-scope-field";
import { UnknownConfiguration } from "@/features/channels/unknown-configuration";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Pointing a scope's runs at a conversation.
 *
 * The scope is the governing field and it is a choice, never typed: a
 * conversation receives the runs of the scope it is configured in and no
 * others, so a free text box here would be a way to name an area somebody
 * cannot otherwise see and have its runs delivered to them.
 */
export function ConversationForm({
  channel,
  conversation,
  onClose,
}: {
  channel: string;
  conversation?: Conversation;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const save = useSaveConversation();
  const stored = conversation?.mode;
  const unsupported = unsupportedConversation(conversation);

  const form = useForm<ConversationValues>({
    resolver: zodResolver(conversationSchema),
    defaultValues: conversationDefaults(conversation),
  });
  const mode = form.watch("mode");
  const scope = form.watch("scope");
  // A conversation for the whole installation hears about every company and
  // starts nothing. Everything inbound is hidden rather than shown and
  // refused: the server strips it, and a field that is saved and forgotten is
  // worse than one that was never offered.
  const reportsOnly = scope === INSTALLATION_SCOPE;
  // A conversation that starts nothing has nothing to say about agents. The
  // installation gets there by its scope; any conversation can get there by
  // choosing the mode, and that one keeps its mode picker — hiding it would
  // make "only reports" a door that only opens one way.
  const startsNothing = reportsOnly || mode === "announce";

  async function submit(values: ConversationValues) {
    try {
      await save.mutateAsync(conversationInput(channel, values));
      toast.success(t("channels.conversationSaved"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  // Before anything is drawn. A form filled with this console's idea of the
  // nearest value is a form whose save rewrites the conversation.
  if (!knownMode(stored) || unsupported.event || unsupported.ticket) {
    return (
      <UnknownConfiguration
        title={t("channels.editConversation")}
        message={
          unsupported.event
            ? t("channels.unknownEvent", { event: unsupported.event })
            : unsupported.ticket
              ? t("channels.unknownTicketRule")
            : t("channels.unknownMode", { mode: stored })
        }
        onClose={onClose}
      />
    );
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        conversation
          ? t("channels.editConversation")
          : t("channels.newConversation")
      }
      description={t("channels.conversationExplains")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <ConversationFormFields
            form={form}
            channel={channel}
            existing={Boolean(conversation)}
            mode={mode}
            scope={scope}
            reportsOnly={reportsOnly}
            startsNothing={startsNothing}
          />

          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {t("common.save")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
