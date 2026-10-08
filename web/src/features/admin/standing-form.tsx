import { zodResolver } from "@hookform/resolvers/zod";
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
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { useActiveScope } from "@/features/scope/active-scope";
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
            <Field
              form={form}
              name="toolId"
              label={t("admin.standingTool")}
              description={t("admin.standingToolHint")}
              mono
            />
            <Field
              form={form}
              name="agentId"
              label={t("admin.standingAgent")}
              mono
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
