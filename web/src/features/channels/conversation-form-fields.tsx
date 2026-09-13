import type { UseFormReturn } from "react-hook-form";
import { PropertiesSheetBody } from "@/components/shared/properties-sheet";
import { ConversationAgentField } from "@/features/channels/conversation-agent-field";
import {
  startsFromMentions,
  startsTickets,
  startsFromWatch,
  type ConversationMode,
  type ConversationValues,
} from "@/features/channels/conversation-form-model";
import { ConversationIdField } from "@/features/channels/conversation-id-field";
import { ConversationModeField } from "@/features/channels/conversation-mode-field";
import { ConversationScopeField } from "@/features/channels/conversation-scope-field";
import { ConversationThreadContextField } from "@/features/channels/conversation-thread-context-field";
import { ConversationTicketFields } from "@/features/channels/conversation-ticket-fields";
import { ConversationWantsField } from "@/features/channels/conversation-wants-field";
import { ConversationWatchFields } from "@/features/channels/conversation-watch-fields";

export function ConversationFormFields({
  form,
  channel,
  existing,
  mode,
  scope,
  reportsOnly,
  startsNothing,
}: {
  form: UseFormReturn<ConversationValues>;
  channel: string;
  existing: boolean;
  mode: ConversationMode;
  scope: string;
  reportsOnly: boolean;
  startsNothing: boolean;
}) {
  return (
    <PropertiesSheetBody className="space-y-4">
      <ConversationIdField form={form} channel={channel} existing={existing} />
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
      {!reportsOnly && startsTickets(mode) && (
        <ConversationTicketFields form={form} />
      )}
    </PropertiesSheetBody>
  );
}
