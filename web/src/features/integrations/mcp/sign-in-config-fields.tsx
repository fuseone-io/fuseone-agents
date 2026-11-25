import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  TOKEN_AUTH_METHODS,
  type SignInConfigValues,
} from "@/features/integrations/mcp/sign-in-config";

type TextField = "clientID" | "clientSecret" | "scopes" | "authorizeURL" | "tokenURL" | "resource";

/** The inputs of a server's browser sign-in: the client, then what is rarely needed under Advanced. */
export function SignInConfigFields({ form }: { form: UseFormReturn<SignInConfigValues> }) {
  const { t } = useTranslation();
  const advancedFilled = ["authorizeURL", "tokenURL", "resource"].some(
    (field) => form.getValues(field as TextField) !== "",
  );
  const [advanced, setAdvanced] = useState(advancedFilled);
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <SignInInput form={form} name="clientID" label={t("mcp.signIn.clientID")} />
        <SignInInput form={form} name="clientSecret" label={t("mcp.signIn.clientSecret")} secret />
      </div>
      <SignInInput form={form} name="scopes" label={t("mcp.signIn.scopes")} />
      <Collapsible open={advanced} onOpenChange={setAdvanced}>
        <CollapsibleTrigger asChild>
          <Button type="button" variant="ghost" size="sm" className="-ml-2">
            <ChevronDown className="size-3.5" />
            {t("mcp.signIn.advanced")}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="grid gap-3 pt-2 sm:grid-cols-2">
          <SignInInput form={form} name="authorizeURL" label={t("mcp.signIn.authorizeURL")} />
          <SignInInput form={form} name="tokenURL" label={t("mcp.signIn.tokenURL")} />
          <SignInInput form={form} name="resource" label={t("mcp.signIn.resource")} />
          <FormField
            control={form.control}
            name="tokenAuthMethod"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("mcp.signIn.tokenAuthMethod")}</FormLabel>
                <Select value={field.value} onValueChange={field.onChange}>
                  <FormControl>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {TOKEN_AUTH_METHODS.map((method) => (
                      <SelectItem key={method} value={method}>
                        {t(`mcp.signIn.method.${method}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormItem>
            )}
          />
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}

function SignInInput({
  form,
  name,
  label,
  secret = false,
}: {
  form: UseFormReturn<SignInConfigValues>;
  name: TextField;
  label: string;
  secret?: boolean;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input {...field} type={secret ? "password" : "text"} autoComplete="off" className="font-mono" />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
