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

const schema = z.object({
  scope: z.string().min(1, "Diga a que este teto se aplica."),
  period: z.enum(["daily", "monthly"]),
  // Entered in currency, stored in micros — the platform never keeps money in
  // a float.
  amount: z
    .string()
    .refine(
      (v) => v === "" || Number(v.replace(",", ".")) > 0,
      "Use um valor maior que zero.",
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
      toast.success("Teto definido", {
        description:
          "Vale a partir da próxima execução; uma que já parou retoma quando houver folga.",
      });
      onClose();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Não foi possível salvar",
      );
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{budget ? "Editar teto" : "Novo teto"}</DialogTitle>
          <DialogDescription>
            Tetos herdam para baixo e nunca ampliam: uma área não levanta o que
            a empresa dela permite. Ao atingir um teto a execução pausa e
            continua de onde parou, em vez de terminar.
          </DialogDescription>
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
                  <FormLabel>Aplica-se a</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      disabled={!!budget}
                      className="font-mono"
                      placeholder="acme/cx"
                    />
                  </FormControl>
                  <FormDescription>
                    <code className="font-mono">installation</code>, uma
                    empresa, ou <code className="font-mono">empresa/área</code>.
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
                  <FormLabel>Janela</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="monthly">
                        Mensal — reinicia no dia 1
                      </SelectItem>
                      <SelectItem value="daily">
                        Diária — reinicia à meia-noite UTC
                      </SelectItem>
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
                  <FormLabel>Teto de gasto</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      inputMode="decimal"
                      className="font-mono"
                      placeholder="500"
                    />
                  </FormControl>
                  <FormDescription>
                    Em reais. Deixe vazio para não limitar por valor.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="steps"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Teto de passos</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      inputMode="numeric"
                      className="font-mono"
                      placeholder="opcional"
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
                  <FormLabel className="m-0">Ativo</FormLabel>
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
                Cancelar
              </Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                Salvar
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
