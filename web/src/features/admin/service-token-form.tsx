import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
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
import {
  useIssueServiceToken,
  type ServiceToken,
} from "@/features/admin/service-accounts-api";
import { ServiceTokenValidity } from "@/features/admin/service-token-validity";
import { problemMessage } from "@/lib/api/problem-message";

const DAY_MS = 24 * 60 * 60 * 1000;

function expiryFrom(days: string): string {
  return new Date(Date.now() + Number(days) * DAY_MS).toISOString();
}

const schema = z.object({
  name: z.string().trim().min(1, "serviceAccounts.tokenNameRequired").max(128),
  days: z.string().min(1, "serviceAccounts.validityRequired"),
});

type Values = z.infer<typeof schema>;

/**
 * Issuing, or rotating when `replacing` is set: the same act, under the same
 * name, beside the token it replaces. Nothing is revoked here, because the
 * machine still has the old one until somebody moves it over. A validity is
 * chosen each time: there is no default, so nobody ends up with one they did
 * not decide on.
 */
export function ServiceTokenForm({
  account,
  replacing,
  onClose,
  onIssued,
}: {
  account: string;
  replacing?: ServiceToken;
  onClose: () => void;
  onIssued: (secret: string) => void;
}) {
  const { t } = useTranslation();
  const issue = useIssueServiceToken(onIssued);
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { name: replacing?.name ?? "", days: "" },
  });

  async function submit(values: Values) {
    try {
      await issue.mutateAsync({
        account,
        name: values.name,
        expiresAt: expiryFrom(values.days),
      });
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        replacing
          ? t("serviceAccounts.rotateTitle", { name: replacing.name })
          : t("serviceAccounts.issue")
      }
      description={
        replacing
          ? t("serviceAccounts.rotateExplains", { id: replacing.id })
          : t("serviceAccounts.issueExplains")
      }
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-4">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("serviceAccounts.tokenName")}</FormLabel>
                  <FormControl>
                    <Input {...field} autoComplete="off" />
                  </FormControl>
                  <FormDescription>
                    {t("serviceAccounts.tokenNameHint")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <ServiceTokenValidity control={form.control} />
          </PropertiesSheetBody>
          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={issue.isPending}>
              {t("serviceAccounts.issueConfirm")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
