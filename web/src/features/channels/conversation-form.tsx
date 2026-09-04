import { zodResolver } from "@hookform/resolvers/zod";
import { RefreshCw } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { usePeople } from "@/features/admin/people-api";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useAvailableConversations,
  useSaveConversation,
} from "@/features/channels/api";
import { ConversationAgentField } from "@/features/channels/conversation-agent-field";
import { useScopes } from "@/features/scope/api";
import { useMe } from "@/features/session/api";
import { problemMessage } from "@/lib/api/problem-message";
import type { components } from "@/lib/api/schema.gen";

const EVENTS = ["parked", "failed", "finished"] as const;
type RunAsPerson = { id: string; display?: string | null; email?: string | null };
type Conversation = components["schemas"]["ChannelConversation"];
type ConversationMode = "mentions" | "watch" | "both";

function splitSources(value: string) {
  return value
    .split(/[\n,]/)
    .map((one) => one.trim())
    .filter(Boolean);
}

const schema = z
  .object({
    conversation: z.string().min(1, "channels.needsConversation"),
    label: z.string(),
    scope: z.string().min(1, "channels.needsScope"),
    mode: z.enum(["mentions", "watch", "both"]),
    threadContext: z.boolean(),
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

export type ConversationValues = z.infer<typeof schema>;

function startsFromMentions(mode: ConversationMode) {
  return mode !== "watch";
}

function startsFromWatch(mode: ConversationMode) {
  return mode === "watch" || mode === "both";
}

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
  const { data: scopes } = useScopes();
  const available = useAvailableConversations(channel);
  const availableItems = available.data?.items ?? [];
  const manuallyEnterConversation =
    available.isError || (available.isSuccess && availableItems.length === 0);

  const form = useForm<ConversationValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      conversation: conversation?.id ?? "",
      scope: conversation
        ? `${conversation.scope.company}/${conversation.scope.area ?? ""}`
        : "",
      label: conversation?.label ?? "",
      mode: conversation?.mode ?? "mentions",
      threadContext: conversation?.threadContext ?? false,
      sources: (conversation?.sources ?? []).join("\n"),
      agent: conversation?.agent ?? "",
      runAs: conversation?.runAs ?? "",
      wants: (conversation?.wants as ("parked" | "failed" | "finished")[]) ?? [
        "parked",
        "failed",
      ],
    },
  });
  const mode = form.watch("mode");
  const scope = form.watch("scope");
  const people = usePeople();
  const peopleItems = (people.data?.items ?? []).filter((p) => !p.disabled);
  const { data: me } = useMe();
  const canDelegateRunAs =
    me === null || Boolean(me?.can.includes("identity:write"));
  const runAsPeople: RunAsPerson[] = canDelegateRunAs
    ? peopleItems
    : me
      ? peopleItems.some((person) => person.id === me.id)
        ? peopleItems.filter((person) => person.id === me.id)
        : [{ id: me.id, display: me.display }]
      : [];

  useEffect(() => {
    if (!startsFromWatch(mode) || me === null || me === undefined) return;
    if (canDelegateRunAs) return;
    if (form.getValues("runAs") === me.id) return;
    form.setValue("runAs", me.id, { shouldValidate: true });
  }, [canDelegateRunAs, form, me, mode]);

  async function submit(values: ConversationValues) {
    // The select's values are always company/area, but a destructure of a
    // split is typed as possibly absent and the compiler is right to say so.
    const [company = "", area = ""] = values.scope.split("/");
    try {
      await save.mutateAsync({
        channel,
        conversation: values.conversation.trim(),
        company,
        area: area || undefined,
        label: values.label.trim() || undefined,
        mode: values.mode,
        threadContext: startsFromMentions(values.mode)
          ? values.threadContext
          : false,
        sources:
          startsFromWatch(values.mode) ? splitSources(values.sources) : undefined,
        agent: values.agent.trim() || undefined,
        runAs: startsFromWatch(values.mode) ? values.runAs.trim() : undefined,
        wants: values.wants,
      });
      toast.success(t("channels.conversationSaved"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
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
            <FormField
              control={form.control}
              name="conversation"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.conversation")}</FormLabel>
                  {conversation ? (
                    <>
                      <FormControl>
                        <Input {...field} disabled className="font-mono" />
                      </FormControl>
                      <FormDescription>
                        {t("channels.conversationIdCannotChange")}
                      </FormDescription>
                    </>
                  ) : manuallyEnterConversation ? (
                    <>
                      {/* Listing is a convenience, not authority. When Slack
                          refuses the list or returns none, the screen says so
                          and lets an operator paste the stable channel id
                          rather than leaving behind a picker with no choices. */}
                      <FormControl>
                        <Input {...field} placeholder="C0123ABCDEF" />
                      </FormControl>
                      <FormDescription
                        className={available.isError ? "text-warning" : ""}
                      >
                        {available.isError
                          ? problemMessage(available.error, t)
                          : t("channels.noAvailableConversations")}
                      </FormDescription>
                      {!available.isError && (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="w-fit"
                          disabled={available.isFetching}
                          onClick={() => void available.refetch()}
                        >
                          <RefreshCw className="size-3.5" aria-hidden />
                          {t("common.retry")}
                        </Button>
                      )}
                    </>
                  ) : (
                    <Select
                      onValueChange={(id) => {
                        field.onChange(id);
                        const picked = availableItems.find((c) => c.id === id);
                        if (picked) form.setValue("label", `#${picked.name}`);
                      }}
                      value={field.value ?? ""}
                      disabled={available.isLoading}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue
                            placeholder={
                              available.isLoading
                                ? t("common.loadingMore")
                                : t("channels.pickConversation")
                            }
                          />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {availableItems.map((c) => (
                          <SelectItem key={c.id} value={c.id}>
                            {c.private ? "🔒 " : "#"}
                            {c.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                  <FormDescription>
                    {t("channels.onlyWhereInvited")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="scope"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("scope.label")}</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value ?? ""}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t("scope.label")} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {(scopes?.items ?? []).map((s) => (
                        <SelectItem
                          key={`${s.company}/${s.area}`}
                          value={`${s.company}/${s.area}`}
                        >
                          {s.label || `${s.company}/${s.area}`}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t("channels.scopeGoverns")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="wants"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.wants")}</FormLabel>
                  <div className="flex flex-col gap-2">
                    {EVENTS.map((event) => (
                      <label
                        key={event}
                        className="flex items-center gap-2 text-sm"
                      >
                        <Checkbox
                          checked={field.value.includes(event)}
                          onCheckedChange={(on) =>
                            field.onChange(
                              on
                                ? [...field.value, event]
                                : field.value.filter((e) => e !== event),
                            )
                          }
                        />
                        {t(`channels.event.${event}`)}
                      </label>
                    ))}
                  </div>
                  <FormDescription>
                    {t("channels.wantsExplains")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="mode"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.startMode")}</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value ?? ""}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="mentions">
                        {t("channels.modeMentions")}
                      </SelectItem>
                      <SelectItem value="watch">
                        {t("channels.modeWatch")}
                      </SelectItem>
                      <SelectItem value="both">
                        {t("channels.modeBoth")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {mode === "watch"
                      ? t("channels.modeWatchExplains")
                      : mode === "both"
                        ? t("channels.modeBothExplains")
                        : t("channels.modeMentionsExplains")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <ConversationAgentField form={form} mode={mode} scope={scope} />
            {startsFromMentions(mode) && (
              <FormField
                control={form.control}
                name="threadContext"
                render={({ field }) => (
                  <FormItem className="rounded-md border bg-muted/30 p-3">
                    <label className="flex items-start gap-2 text-sm">
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={(on) => field.onChange(Boolean(on))}
                      />
                      <span className="grid gap-1">
                        <span className="font-medium">
                          {t("channels.threadContext")}
                        </span>
                        <span className="text-muted-foreground">
                          {t("channels.threadContextExplains")}
                        </span>
                      </span>
                    </label>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            {startsFromWatch(mode) && (
              <div className="rounded-md border bg-muted/30 p-3">
                <div className="grid gap-4">
                  <FormField
                    control={form.control}
                    name="runAs"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t("channels.watchRunAs")}</FormLabel>
                        {runAsPeople.length > 0 ? (
                          <Select
                            onValueChange={field.onChange}
                            value={field.value ?? ""}
                          >
                            <FormControl>
                              <SelectTrigger>
                                <SelectValue
                                  placeholder={t("channels.pickWatchRunAs")}
                                />
                              </SelectTrigger>
                            </FormControl>
                            <SelectContent>
                              {runAsPeople.map((person) => (
                                <SelectItem key={person.id} value={person.id}>
                                  {person.display || person.email || person.id}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        ) : me === undefined ? (
                          <Select disabled value="">
                            <FormControl>
                              <SelectTrigger>
                                <SelectValue
                                  placeholder={t("common.loading")}
                                />
                              </SelectTrigger>
                            </FormControl>
                          </Select>
                        ) : (
                          <FormControl>
                            <Input
                              {...field}
                              placeholder={t("channels.watchRunAsPlaceholder")}
                            />
                          </FormControl>
                        )}
                        <FormDescription>
                          {canDelegateRunAs
                            ? t("channels.watchRunAsExplains")
                            : t("channels.watchRunAsSelfOnly")}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <FormField
                  control={form.control}
                  name="sources"
                  render={({ field }) => (
                    <FormItem className="mt-4">
                      <FormLabel>{t("channels.watchSources")}</FormLabel>
                      <FormControl>
                        <Textarea
                          {...field}
                          className="min-h-20 font-mono text-xs"
                          placeholder={t("channels.watchSourcesPlaceholder")}
                        />
                      </FormControl>
                      <FormDescription>
                        {t("channels.watchSourcesExplains")}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
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
