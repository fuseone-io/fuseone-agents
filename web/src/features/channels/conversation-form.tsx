import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useSaveConversation } from "@/features/channels/api";
import { useScopes } from "@/features/scope/api";
import { problemMessage } from "@/lib/api/problem-message";

const EVENTS = ["parked", "failed", "finished"] as const;

const schema = z.object({
  conversation: z.string().min(1, "channels.needsConversation"),
  scope: z.string().min(1, "channels.needsScope"),
  label: z.string(),
  wants: z.array(z.enum(EVENTS)).min(1, "channels.needsEvent"),
});

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
  onClose,
}: {
  channel: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const save = useSaveConversation();
  const { data: scopes } = useScopes();

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      conversation: "",
      scope: "",
      label: "",
      wants: ["parked", "failed"],
    },
  });

  async function submit(values: z.infer<typeof schema>) {
    const [company, area = ""] = values.scope.split("/");
    try {
      await save.mutateAsync({
        channel,
        conversation: values.conversation.trim(),
        company,
        area: area || undefined,
        label: values.label.trim() || undefined,
        wants: values.wants,
      });
      toast.success(t("channels.conversationSaved"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("channels.newConversation")}</DialogTitle>
          <DialogDescription>
            {t("channels.conversationExplains")}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <FormField
              control={form.control}
              name="conversation"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("channels.conversationId")}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="C0123ABCDEF" />
                  </FormControl>
                  <FormDescription>
                    {t("channels.conversationIdExplains")}
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
                  <Select onValueChange={field.onChange} value={field.value}>
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
              name="label"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.shownAs")}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="#alertas" />
                  </FormControl>
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

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
