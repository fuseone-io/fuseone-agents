import { z } from "zod";

/*
What a conversation form holds, apart from how it is drawn.

The schema, the values and the two predicates live away from the screen because
every field file needs them and none of them may import the screen back — and
because the rules are worth reading on their own, without four hundred lines of
JSX around them.
*/

// What the console offers, and what a new conversation starts with. Drift is
// among the defaults for the reason the platform makes it one: it fires rarely,
// and an agent that quietly stopped holding its corrections is precisely the
// thing nobody thinks to go and ask about.
//
// The stored set is wider by one: a conversation for the whole installation is
// stored as `announce`, chosen by its scope rather than picked from a list.
export const EVENTS = ["parked", "failed", "finished", "drifted"] as const;
export const EVENTS_BY_DEFAULT = ["parked", "failed", "drifted"] as const;

// The scope above every company, written the way the API writes it. A
// conversation there hears about a run in any company, which is what makes it
// the answer for an area nobody has pointed at a channel of its own.
export const INSTALLATION_SCOPE = "*/";

// Every mode that can be stored, including the one nobody picks from the
// events list. A value this console does not know would be refused by the
// server rather than silently read as mentions.
export type ConversationMode = "mentions" | "watch" | "both" | "announce";

/*
knownMode answers whether this console can draw a stored mode at all.

A conversation configured by a newer version travels back as itself now, which
is what stopped an unrelated edit from rewriting it as "mentions". The console
has to do its half: a value it cannot draw is a value it must not offer to
save.
*/
export function knownMode(
  mode: string | undefined,
): mode is ConversationMode | undefined {
  return (
    mode === undefined ||
    mode === "mentions" ||
    mode === "watch" ||
    mode === "both" ||
    mode === "announce"
  );
}

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
    // A conversation for the whole installation starts nothing, whatever the
    // hidden mode field still says. Validating it anyway trapped a half-written
    // watch rule in the form: three fields the screen was no longer showing
    // refused the save, with nothing to fix and nowhere to fix it.
    if (value.scope === INSTALLATION_SCOPE) return;
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
