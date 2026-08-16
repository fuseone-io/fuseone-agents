import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import type { ServerFormValues } from "@/features/integrations/server-schema";

/**
 * The half of the form that depends on how the server is reached.
 *
 * A local server is a command this installation runs; a remote one is an
 * address it calls. Showing both at once would invite filling both, and only
 * one of them is ever used.
 */
export function ServerFields({
  form,
  hasSecret,
}: {
  form: UseFormReturn<ServerFormValues>;
  hasSecret: boolean;
}) {
  const { t } = useTranslation();

  if (form.watch("transport") === "http") {
    return (
      <>
        <Field
          form={form}
          name="url"
          label={t("integrations.url")}
          placeholder="https://api.example.com/mcp/"
        />
        <Field
          form={form}
          name="token"
          label={t("integrations.token")}
          placeholder=""
          hint={
            hasSecret
              ? t("integrations.tokenKept")
              : t("integrations.tokenHint")
          }
        />
      </>
    );
  }

  return (
    <>
      <Field
        form={form}
        name="command"
        label={t("integrations.command")}
        placeholder="/usr/local/bin/crm-mcp"
        hint={t("integrations.commandRuns")}
      />
      <Field
        form={form}
        name="args"
        label={t("integrations.arguments")}
        placeholder="--config /etc/crm.yaml"
      />
      <AcceptLocalExecution form={form} />
    </>
  );
}

/**
 * What a local server is, said where the decision is made.
 *
 * Not a warning under the heading. A local server is a program this
 * installation starts inside the worker: it runs as the worker, on its
 * filesystem, from inside its network, and the Gate decides what a tool may do
 * while deciding nothing about what a process may read.
 *
 * A checkbox does not stop an administrator who means it, and pretending
 * otherwise would be worse than having none. What it does is make the
 * difference between the two transports impossible to pass without reading,
 * and put a name against the answer.
 */
function AcceptLocalExecution({
  form,
}: {
  form: UseFormReturn<ServerFormValues>;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={form.control}
      name="acceptsLocalExecution"
      render={({ field }) => (
        <FormItem className="rounded-lg border border-destructive/40 bg-destructive/5 p-3">
          <div className="flex items-start gap-3">
            <FormControl>
              <Checkbox
                checked={field.value}
                onCheckedChange={field.onChange}
                className="mt-0.5"
              />
            </FormControl>
            <div className="space-y-1">
              <FormLabel className="m-0">
                {t("integrations.acceptLocalExecution")}
              </FormLabel>
              <FormDescription>
                {t("integrations.acceptLocalExecutionWhy")}
              </FormDescription>
            </div>
          </div>
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
  placeholder,
  hint,
}: {
  form: UseFormReturn<ServerFormValues>;
  name: "url" | "token" | "command" | "args";
  label: string;
  placeholder: string;
  hint?: string;
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
              className="font-mono"
              placeholder={placeholder}
              autoComplete="off"
              type={name === "token" ? "password" : undefined}
            />
          </FormControl>
          {hint && <FormDescription>{hint}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
