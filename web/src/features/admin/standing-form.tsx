import { zodResolver } from "@hookform/resolvers/zod";
import { Check, ChevronsUpDown } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Textarea } from "@/components/ui/textarea";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { useTools } from "@/features/admin/api";
import { useAgents } from "@/features/agents/api";
import { useActiveScope } from "@/features/scope/active-scope";
import { cn } from "@/lib/utils";
import { useCreateStandingApproval } from "@/features/admin/standing-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Granting ahead of time is a decision, so the form demands what a decision
 * has: one exact tool (a pattern is refused — a mandate must know what it
 * covers), one agent, a ceiling, a reason, and a life of at most 90 days.
 */
const schema = z.object({
  toolId: z
    .string()
    .min(1, "admin.standingToolRequired")
    .refine((value) => !/[*?[]/.test(value), "admin.standingToolExact"),
  agentId: z.string().min(1, "admin.standingAgentRequired"),
  company: z.string().min(1, "connectors.companyRequired"),
  area: z.string(),
  dailyCap: z.coerce.number().int().min(1).max(100),
  expiresInDays: z.coerce.number().int().min(1).max(90),
  reason: z.string().min(1, "admin.standingReasonRequired"),
});

type Values = z.infer<typeof schema>;

function expiryFrom(days: number): string {
  return new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString();
}

export function StandingForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const company = useActiveScope((s) => s.company);
  const area = useActiveScope((s) => s.area);
  const create = useCreateStandingApproval();
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    mode: "onChange",
    defaultValues: {
      toolId: "",
      agentId: "",
      company,
      area,
      dailyCap: 5,
      expiresInDays: 30,
      reason: "",
    },
  });

  async function submit(values: Values) {
    try {
      await create.mutateAsync({
        toolId: values.toolId.trim(),
        agentId: values.agentId.trim(),
        scope: { company: values.company.trim(), area: values.area.trim() },
        dailyCap: values.dailyCap,
        reason: values.reason.trim(),
        expiresAt: expiryFrom(values.expiresInDays),
      });
      toast.success(t("admin.standingGranted"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("admin.standingNew")}
      description={t("admin.standingSheetHint")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-4">
            <SuggestedField
              form={form}
              name="toolId"
              label={t("admin.standingTool")}
              description={t("admin.standingToolHint")}
              suggest={t("admin.standingSuggestTools")}
              useSuggestions={useToolSuggestions}
            />
            <SuggestedField
              form={form}
              name="agentId"
              label={t("admin.standingAgent")}
              suggest={t("admin.standingSuggestAgents")}
              useSuggestions={useAgentSuggestions}
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <Field form={form} name="company" label={t("scope.label")} mono />
              <Field form={form} name="area" label={t("admin.area")} mono />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field
                form={form}
                name="dailyCap"
                label={t("admin.standingDailyCap")}
                type="number"
              />
              <Field
                form={form}
                name="expiresInDays"
                label={t("admin.standingExpiresIn")}
                description={t("admin.standingExpiresHint")}
                type="number"
              />
            </div>
            <FormField
              control={form.control}
              name="reason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.standingReason")}</FormLabel>
                  <FormControl>
                    <Textarea {...field} className="min-h-20" />
                  </FormControl>
                  <FormDescription>
                    {t("admin.standingReasonHint")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </PropertiesSheetBody>
          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={form.formState.isSubmitting}>
              {t("admin.standingGrant")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}

type Suggestion = { value: string; hint: string };

// The listings are asked for lazily, when the suggestions open; the page is
// an admin's, but the idea generalises and lazy costs nothing.
function useToolSuggestions(open: boolean): Suggestion[] {
  const tools = useTools(open);
  return (tools.data?.items ?? []).map((tool) => ({
    value: tool.toolId,
    hint: tool.effect ?? "",
  }));
}

function useAgentSuggestions(open: boolean): Suggestion[] {
  const agents = useAgents();
  return open
    ? (agents.data?.items ?? []).map((agent) => ({
        value: agent.agentId,
        hint: agent.name,
      }))
    : [];
}

/**
 * An input whose suggestions come from what the platform governs. A mandate
 * demands an exact id, which is exactly where typing from memory fails —
 * but typing stays valid: a grant may name a tool an operator is about to
 * configure.
 */
function SuggestedField({
  form,
  name,
  label,
  description,
  suggest,
  useSuggestions,
}: {
  form: ReturnType<typeof useForm<Values>>;
  name: "toolId" | "agentId";
  label: string;
  description?: string;
  suggest: string;
  useSuggestions: (open: boolean) => Suggestion[];
}) {
  const [open, setOpen] = useState(false);
  const suggestions = useSuggestions(open);
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <div className="flex min-w-0 items-center gap-1">
            <FormControl>
              <Input
                {...field}
                autoComplete="off"
                className="min-w-0 flex-1 font-mono"
              />
            </FormControl>
            <Popover open={open} onOpenChange={setOpen}>
              <PopoverTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  role="combobox"
                  aria-expanded={open}
                  aria-label={suggest}
                  className="size-9 shrink-0"
                >
                  <ChevronsUpDown className="size-3.5 opacity-60" aria-hidden />
                </Button>
              </PopoverTrigger>
              <PopoverContent align="end" className="w-96 p-0">
                <Command>
                  <CommandInput className="font-mono" />
                  <CommandList>
                    <CommandEmpty className="p-2 text-xs text-muted-foreground">
                      {suggest}
                    </CommandEmpty>
                    <CommandGroup>
                      {suggestions.map((one) => (
                        <CommandItem
                          key={one.value}
                          value={one.value}
                          className="font-mono text-xs"
                          onSelect={() => {
                            field.onChange(one.value);
                            setOpen(false);
                          }}
                        >
                          <Check
                            className={cn(
                              "size-3.5",
                              one.value === field.value
                                ? "opacity-100"
                                : "opacity-0",
                            )}
                          />
                          <span className="truncate">{one.value}</span>
                          {one.hint && (
                            <span className="ml-auto truncate pl-2 text-muted-foreground">
                              {one.hint}
                            </span>
                          )}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
          </div>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function Field({
  form,
  name,
  label,
  description,
  type = "text",
  mono,
}: {
  form: ReturnType<typeof useForm<Values>>;
  name:
    "toolId" | "agentId" | "company" | "area" | "dailyCap" | "expiresInDays";
  label: string;
  description?: string;
  type?: string;
  mono?: boolean;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              type={type}
              autoComplete="off"
              className={mono ? "font-mono" : undefined}
            />
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
