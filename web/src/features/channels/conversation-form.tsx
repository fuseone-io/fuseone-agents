import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { useSaveConversation } from "@/features/channels/api";
import { ConversationAgentField } from "@/features/channels/conversation-agent-field";
import {
  conversationSchema,
  EVENTS_BY_DEFAULT,
  knownEvent,
  knownMode,
  splitSources,
  startsFromMentions,
  startsFromWatch,
  type ConversationValues,
} from "@/features/channels/conversation-form-model";
import { ConversationIdField } from "@/features/channels/conversation-id-field";
import { ConversationModeField } from "@/features/channels/conversation-mode-field";
import {
  ConversationScopeField,
  INSTALLATION_SCOPE,
} from "@/features/channels/conversation-scope-field";
import { ConversationThreadContextField } from "@/features/channels/conversation-thread-context-field";
import { UnknownConfiguration } from "@/features/channels/unknown-configuration";
import { ConversationWantsField } from "@/features/channels/conversation-wants-field";
import { ConversationWatchFields } from "@/features/channels/conversation-watch-fields";
import { problemMessage } from "@/lib/api/problem-message";
import type { components } from "@/lib/api/schema.gen";

type Conversation = components["schemas"]["ChannelConversation"];

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
  // What is stored, which since it started travelling back as itself may be a
  // value this console cannot draw. Narrowed here and refused below; the form
  // never sees anything but a mode it knows.
  const stored = conversation?.mode;
  const startMode = knownMode(stored) ? (stored ?? "mentions") : "mentions";
  // The same for the events. One this console cannot name is in no checkbox,
  // so a form drawn anyway would show a conversation asking for less than it
  // asks for, and save exactly that.
  const strangeEvent = (conversation?.wants ?? []).find(
    (one) => !knownEvent(one),
  );

  const form = useForm<ConversationValues>({
    resolver: zodResolver(conversationSchema),
    defaultValues: {
      conversation: conversation?.id ?? "",
      scope: conversation
        ? `${conversation.scope.company}/${conversation.scope.area ?? ""}`
        : "",
      label: conversation?.label ?? "",
      mode: startMode,
      threadContext: conversation?.threadContext ?? false,
      directApprovals: conversation?.directApprovals ?? false,
      sources: (conversation?.sources ?? []).join("\n"),
      agent: conversation?.agent ?? "",
      runAs: conversation?.runAs ?? "",
      wants: (conversation?.wants as ConversationValues["wants"]) ?? [
        ...EVENTS_BY_DEFAULT,
      ],
    },
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
    // The select's values are always company/area, but a destructure of a
    // split is typed as possibly absent and the compiler is right to say so.
    const [company = "", area = ""] = values.scope.split("/");
    // One mode decides the whole request. A conversation for the installation
    // reports and starts nothing, so every field about starting is sent as
    // absent rather than left to be stripped on the far side — a request that
    // says one thing while the server stores another is what makes the console
    // disagree with itself on the next read.
    const picked = reportsOnly ? "announce" : values.mode;
    try {
      await save.mutateAsync({
        channel,
        conversation: values.conversation.trim(),
        company,
        area: area || undefined,
        label: values.label.trim() || undefined,
        mode: picked,
        directApprovals:
          values.wants.includes("parked") && values.directApprovals,
        threadContext: startsFromMentions(picked)
          ? values.threadContext
          : false,
        sources: startsFromWatch(picked)
          ? splitSources(values.sources)
          : undefined,
        agent: startsNothing ? undefined : values.agent.trim() || undefined,
        runAs: startsFromWatch(picked) ? values.runAs.trim() : undefined,
        wants: values.wants,
      });
      toast.success(t("channels.conversationSaved"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  // Before anything is drawn. A form filled with this console's idea of the
  // nearest value is a form whose save rewrites the conversation.
  if (!knownMode(stored) || strangeEvent) {
    return (
      <UnknownConfiguration
        title={t("channels.editConversation")}
        message={
          strangeEvent
            ? t("channels.unknownEvent", { event: strangeEvent })
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
          <PropertiesSheetBody className="space-y-4">
            <ConversationIdField
              form={form}
              channel={channel}
              existing={Boolean(conversation)}
            />
            <ConversationScopeField form={form} />
            <ConversationWantsField form={form} />
            {!reportsOnly && <ConversationModeField form={form} mode={mode} />}
            {!startsNothing && (
              <ConversationAgentField form={form} mode={mode} scope={scope} />
            )}
            {!reportsOnly && startsFromMentions(mode) && (
              <ConversationThreadContextField form={form} />
            )}
            {!reportsOnly && startsFromWatch(mode) && (
              <ConversationWatchFields form={form} />
            )}
          </PropertiesSheetBody>

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
