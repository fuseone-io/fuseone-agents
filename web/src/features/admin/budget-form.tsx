import { Trans, useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { usePutBudget, type ScopeBudget } from "@/features/admin/api";
import { scopePath } from "@/features/admin/budget-scope";
import { problemMessage } from "@/lib/api/problem-message";

const schema = z.object({
  scope: z.string().min(1, "admin.sayScope"),
  period: z.enum(["daily", "monthly"]),
  // Entered in currency, stored in micros — the platform never keeps money in
  // a float.
  amount: z
    .string()
    .refine(
      (v) => v === "" || Number(v.replace(",", ".")) > 0,
      "admin.aboveZero",
    ),
  steps: z.string(),
  enabled: z.boolean(),
});

export function BudgetForm({
  budget,
  onClose,
}: {
  budget: ScopeBudget | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const put = usePutBudget();
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      scope: budget ? scopePath(budget) : "",
      period: (budget?.period as "daily" | "monthly") ?? "monthly",
      amount: budget?.micros ? String(budget.micros / 1_000_000) : "",
      steps: budget?.steps ? String(budget.steps) : "",
      enabled: budget?.enabled ?? true,
    },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      await put.mutateAsync({
        scope: values.scope.trim(),
        period: values.period,
        micros: values.amount
          ? Math.round(Number(values.amount.replace(",", ".")) * 1_000_000)
          : undefined,
        steps: values.steps ? Number(values.steps) : undefined,
        enabled: values.enabled,
      });
      toast.success(t("admin.ceilingSet"), {
        description: t("admin.ceilingApplies"),
      });
      onClose();
    } catch (error) {
      toast.error(
        problemMessage(error, t),
      );
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {budget ? t("admin.editCeiling") : t("admin.newCeiling")}
          </DialogTitle>
          <DialogDescription>{t("admin.ceilingsInherit")}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <FormField
              control={form.control}
              name="scope"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.appliesTo")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      disabled={!!budget}
                      className="font-mono"
                      placeholder="acme/cx"
                    />
                  </FormControl>
                  <FormDescription>
                    <Trans
                      i18nKey="admin.scopeShapes"
                      components={{ code: <code className="font-mono" /> }}
                    />
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="period"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.window")}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="monthly">
                        {t("admin.monthly")}
                      </SelectItem>
                      <SelectItem value="daily">{t("admin.daily")}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="amount"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.spendCeiling")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      inputMode="decimal"
                      className="font-mono"
                      placeholder="500"
                    />
                  </FormControl>
                  <FormDescription>{t("admin.inCurrency")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="steps"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.stepCeiling")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      inputMode="numeric"
                      className="font-mono"
                      placeholder={t("common.optional")}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="enabled"
              render={({ field }) => (
                <FormItem className="flex items-center justify-between rounded-lg border p-3">
                  <FormLabel className="m-0">
                    {t("integrations.enabled")}
                  </FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
