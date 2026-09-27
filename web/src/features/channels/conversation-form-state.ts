import {
  EVENTS_BY_DEFAULT,
  knownEvent,
  knownMode,
  knownTicketPolicy,
  marksThreads,
  splitSources,
  splitClosingEmoji,
  splitTicketPatterns,
  startsFromMentions,
  startsTickets,
  startsFromWatch,
  type ConversationValues,
} from "@/features/channels/conversation-form-model";
import type { ConversationInput } from "@/features/channels/api";
import { INSTALLATION_SCOPE } from "@/features/channels/conversation-scope-field";
import type { components } from "@/lib/api/schema.gen";

export type Conversation = components["schemas"]["ChannelConversation"];

export function conversationDefaults(
  conversation: Conversation | undefined,
): ConversationValues {
  const stored = conversation?.mode;
  return {
    conversation: conversation?.id ?? "",
    scope: conversation
      ? `${conversation.scope.company}/${conversation.scope.area ?? ""}`
      : "",
    label: conversation?.label ?? "",
    mode: knownMode(stored) ? (stored ?? "mentions") : "mentions",
    threadContext: conversation?.threadContext ?? false,
    directApprovals: conversation?.directApprovals ?? false,
    finishedAnswerOnly: conversation?.finishedAnswerOnly ?? false,
    formattedAnswers: conversation?.formattedAnswers ?? false,
    sources: (conversation?.sources ?? []).join("\n"),
    agent: conversation?.agent ?? "",
    runAs: conversation?.runAs ?? "",
    ticketOpenFrom: knownTicketPolicy(conversation?.ticket?.openFrom)
      ? conversation.ticket.openFrom
      : "linked_users",
    ticketRootFrom: conversation?.ticket?.rootFrom ?? "",
    ticketReviewIn: conversation?.ticket?.reviewIn ?? "",
    ticketClosesOn: (conversation?.ticket?.closesOn ?? []).join(" "),
    ticketAddressFrom: conversation?.ticket?.addressFrom ?? "",
    ticketPatterns: (conversation?.ticket?.patterns ?? []).join("\n"),
    wants: (conversation?.wants as ConversationValues["wants"]) ?? [
      ...EVENTS_BY_DEFAULT,
    ],
  };
}

export function unsupportedConversation(
  conversation: Conversation | undefined,
) {
  const mode = conversation?.mode;
  const event = (conversation?.wants ?? []).find((one) => !knownEvent(one));
  return {
    mode: !knownMode(mode) ? mode : undefined,
    event,
    ticket: mode === "ticket" && !knownTicketPolicy(conversation?.ticket?.openFrom),
  };
}

export function conversationInput(
  channel: string,
  values: ConversationValues,
): ConversationInput {
  const [company = "", area = ""] = values.scope.split("/");
  const reportsOnly = values.scope === INSTALLATION_SCOPE;
  const mode = reportsOnly ? "announce" : values.mode;
  const startsNothing = reportsOnly || mode === "announce";
  return {
    channel,
    conversation: values.conversation.trim(),
    company,
    area: area || undefined,
    label: values.label.trim() || undefined,
    mode,
    directApprovals: values.wants.includes("parked") && values.directApprovals,
    finishedAnswerOnly:
      values.wants.includes("finished") && values.finishedAnswerOnly,
    formattedAnswers:
      values.wants.includes("finished") && values.formattedAnswers,
    threadContext: startsFromMentions(mode) ? values.threadContext : false,
    sources: startsFromWatch(mode) ? splitSources(values.sources) : undefined,
    agent: startsNothing ? undefined : values.agent.trim() || undefined,
    ticket: startsTickets(mode)
      ? {
          openFrom: values.ticketOpenFrom,
          // Sent only by the policy that reads it: a root source under
          // `linked_users` is a rule the server refuses, and rightly.
          ...(marksThreads(values.ticketOpenFrom)
            ? { rootFrom: values.ticketRootFrom.trim() }
            : {}),
          ...(values.ticketReviewIn.trim()
            ? { reviewIn: values.ticketReviewIn.trim() }
            : {}),
          ...(splitClosingEmoji(values.ticketClosesOn).length
            ? { closesOn: splitClosingEmoji(values.ticketClosesOn) }
            : {}),
          addressFrom: values.ticketAddressFrom.trim(),
          patterns: splitTicketPatterns(values.ticketPatterns),
        }
      : undefined,
    runAs:
      startsFromWatch(mode) || startsTickets(mode)
        ? values.runAs.trim()
        : undefined,
    wants: values.wants,
  };
}
