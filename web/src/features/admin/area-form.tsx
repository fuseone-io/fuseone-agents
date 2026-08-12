import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import { useRegisterScope } from "@/features/scope/api";
import { useMe } from "@/features/session/api";

const schema = z.object({
  company: z.string().min(1, "admin.sayCompany"),
  name: z.string().min(1, "admin.areaNeedsName"),
  label: z.string(),
});

/**
 * Declares an area.
 *
 * The name is folded server-side and the reply carries the id it became, which
 * is what the toast reports: somebody who types "Risco de Crédito" has to be
 * told the platform will call it `risco-de-credito`, because that is the
 * string they will type into a ceiling and read in a policy.
 */
export function AreaForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const register = useRegisterScope();
  const { data: me } = useMe();
  const companies = [...new Set(me?.grants.map((g) => g.company) ?? [])];

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { company: companies[0] ?? "", name: "", label: "" },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      const created = await register.mutateAsync({
        company: values.company.trim(),
        name: values.name.trim(),
        label: values.label.trim() || undefined,
      });
      toast.success(`Área ${created.label || created.area} declarada`, {
        description: `A plataforma a chama de ${created.company}/${created.area}.`,
      });
      onClose();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t("admin.declareFailed"),
      );
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("admin.newArea")}</DialogTitle>
          <DialogDescription>{t("admin.areaExplains")}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <FormField
              control={form.control}
              name="company"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.company")}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="default" />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.name")}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t("admin.areaExample")} />
                  </FormControl>
                  <FormDescription>{t("admin.areaFolds")}</FormDescription>
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
                    <Input {...field} placeholder="opcional" />
                  </FormControl>
                  <FormDescription>{t("admin.emptyUsesName")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={register.isPending}>
                {t("admin.declare")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
