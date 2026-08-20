import { useTranslation } from "react-i18next";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, type ControllerRenderProps } from "react-hook-form";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { currentCurrency } from "@/lib/format";
import { useMoney, useSetMoney } from "@/features/money/api";

const currencyPattern = /^[A-Z]{3}$/;

const schema = z.object({
  currency: z.string().refine((v) => {
    return currencyPattern.test(v.trim().toUpperCase());
  }, "Use a three-letter currency code."),
});

type CurrencyFormValues = z.infer<typeof schema>;

export function PriceCurrencyControl() {
  const money = useMoney();
  const currency = money.data?.currency ?? currentCurrency();
  return <CurrencyForm key={currency} currency={currency} />;
}

function CurrencyForm({ currency }: { currency: string }) {
  const { t } = useTranslation();
  const save = useSetMoney();
  const form = useForm<CurrencyFormValues>({
    resolver: zodResolver(schema),
    mode: "onChange",
    defaultValues: { currency },
  });

  function submit(values: CurrencyFormValues) {
    const next = values.currency.trim().toUpperCase();
    save.mutate(
      { currency: next },
      {
        onSuccess: () => toast.success(t("admin.currencySaved")),
        onError: (e) =>
          toast.error(t("admin.currencySaveFailed"), {
            description: e instanceof Error ? e.message : undefined,
          }),
      },
    );
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(submit)}
        className="flex items-center gap-2 rounded-md border border-border bg-card px-2 py-1"
      >
        <span className="text-xs text-muted-foreground">
          {t("admin.currency")}
        </span>
        <FormField
          control={form.control}
          name="currency"
          render={({ field }) => {
            const state = currencyState(field.value, currency);
            return (
              <>
                <CurrencyField field={field} invalid={state.invalid} />
                <Button
                  type="submit"
                  size="sm"
                  variant="ghost"
                  className="h-7 px-2"
                  disabled={state.invalid || state.unchanged || save.isPending}
                >
                  {t("common.save")}
                </Button>
              </>
            );
          }}
        />
      </form>
    </Form>
  );
}

function CurrencyField({
  field,
  invalid,
}: {
  field: ControllerRenderProps<CurrencyFormValues, "currency">;
  invalid: boolean;
}) {
  const { t } = useTranslation();
  return (
    <FormItem>
      <FormControl>
        <Input
          {...field}
          value={field.value}
          onChange={(event) => field.onChange(event.target.value.toUpperCase())}
          maxLength={3}
          className="h-7 w-16 font-mono uppercase"
          aria-label={t("admin.installationCurrency")}
          aria-invalid={invalid || undefined}
        />
      </FormControl>
    </FormItem>
  );
}

function currencyState(value: string, current?: string) {
  const normalized = value.trim().toUpperCase();
  return {
    invalid: !currencyPattern.test(normalized),
    unchanged: current === normalized,
  };
}
