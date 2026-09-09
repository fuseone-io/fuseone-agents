import { z } from "zod";

/*
What a conversation form holds, apart from how it is drawn.

The schema, the values and the two predicates live away from the screen because
every field file needs them and none of them may import the screen back — and
because the rules are worth reading on their own, without four hundred lines of
JSX around them.
*/

// What the console offers. The stored set is wider: a conversation for the
// whole installation is stored as `announce`, chosen by its scope rather than
// picked from a list, and drift reaches only a conversation stored with no
// choice at all.
export const EVENTS = ["parked", "failed", "finished"] as const;

// Every mode that can be stored, including the one nobody picks from the
// events list. A value this console does not know would be refused by the
// server rather than silently read as mentions.
export type ConversationMode = "mentions" | "watch" | "both" | "announce";

export function splitSources(value: string) {
  return value
    .split(/[\n,]/)
    .map((one) => one.trim())
    .filter(Boolean);
}

/*
 * Which modes let a message start a run, said the same way the server says it.
 *
 * An allowlist. Written as "anything that is not watch", a mode added later is
 * answered yes — so `announce`, whose whole purpose is to start nothing, would
 * have shown the mention fields and offered to configure an inbound path the
 * server refuses.
 */
export function startsFromMentions(mode: ConversationMode) {
  return mode === "mentions" || mode === "both";
}

export function startsFromWatch(mode: ConversationMode) {
  return mode === "watch" || mode === "both";
}

export const conversationSchema = z
  .object({
    conversation: z.string().min(1, "channels.needsConversation"),
    label: z.string(),
    scope: z.string().min(1, "channels.needsScope"),
    mode: z.enum(["mentions", "watch", "both", "announce"]),
    threadContext: z.boolean(),
    directApprovals: z.boolean(),
    sources: z.string(),
    agent: z.string(),
    runAs: z.string(),
    wants: z.array(z.enum(EVENTS)).min(1, "channels.needsEvent"),
  })
  .superRefine((value, ctx) => {
    if (!startsFromWatch(value.mode)) return;
    if (splitSources(value.sources).length === 0) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["sources"],
        message: "channels.needsWatchSource",
      });
    }
    if (value.agent.trim() === "") {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["agent"],
        message: "channels.needsWatchAgent",
      });
    }
    if (value.runAs.trim() === "") {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["runAs"],
        message: "channels.needsWatchRunAs",
      });
    }
  });

export type ConversationValues = z.infer<typeof conversationSchema>;
