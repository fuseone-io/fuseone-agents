import { Plus, Trash2 } from "lucide-react";
import { useFieldArray, type UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
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
import { Textarea } from "@/components/ui/textarea";
import {
  emptySQLTemplate,
  type SQLInstanceValues,
} from "@/features/integrations/connectors/sql-instance-model";

export function SQLTemplateEditor({
  form,
}: {
  form: UseFormReturn<SQLInstanceValues>;
}) {
  const { t } = useTranslation();
  const templates = useFieldArray({ control: form.control, name: "templates" });
  return (
    <section className="grid gap-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-medium">{t("connectors.sqlTemplates")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("connectors.sqlTemplatesHint")}
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={templates.fields.length >= 64}
          onClick={() => templates.append(emptySQLTemplate())}
        >
          <Plus className="size-4" aria-hidden />
          {t("connectors.addSQLTemplate")}
        </Button>
      </div>
      {templates.fields.map((template, index) => (
        <SQLTemplateFields
          key={template.id}
          form={form}
          index={index}
          onRemove={() => templates.remove(index)}
        />
      ))}
      {templates.fields.length === 0 && (
        <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t("connectors.noSQLTemplates")}
        </p>
      )}
    </section>
  );
}

function SQLTemplateFields({
  form,
  index,
  onRemove,
}: {
  form: UseFormReturn<SQLInstanceValues>;
  index: number;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const parameters = useFieldArray({
    control: form.control,
    name: `templates.${index}.parameters`,
  });
  const id = form.watch(`templates.${index}.id`);
  return (
    <div className="grid gap-4 rounded-md border p-4">
      <div className="flex items-center gap-3">
        <h4 className="min-w-0 flex-1 truncate text-sm font-medium">
          {id || t("connectors.sqlTemplateUntitled", { number: index + 1 })}
        </h4>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          onClick={onRemove}
          aria-label={t("connectors.removeSQLTemplate", { number: index + 1 })}
        >
          <Trash2 className="size-4" aria-hidden />
        </Button>
      </div>
      <FormField
        control={form.control}
        name={`templates.${index}.id`}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.sqlTemplateId")}</FormLabel>
            <FormControl>
              <Input {...field} className="font-mono" autoComplete="off" />
            </FormControl>
            <FormDescription>{t("connectors.sqlTemplateIdHint")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={`templates.${index}.sql`}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.sqlQuery")}</FormLabel>
            <FormControl>
              <Textarea {...field} className="min-h-32 font-mono text-xs" spellCheck={false} />
            </FormControl>
            <FormDescription>{t("connectors.sqlQueryHint")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <div className="grid gap-3 sm:grid-cols-3">
        <TemplateNumberField
          form={form}
          name={`templates.${index}.timeoutSeconds`}
          label={t("connectors.sqlTimeout")}
          min={1}
          max={3600}
        />
        <TemplateNumberField
          form={form}
          name={`templates.${index}.maxRows`}
          label={t("connectors.sqlMaxRows")}
          min={1}
          max={10_000}
        />
        <TemplateNumberField
          form={form}
          name={`templates.${index}.maxBytes`}
          label={t("connectors.sqlMaxBytes")}
          min={1024}
          max={1_048_576}
        />
      </div>
      <div className="grid gap-2 border-t pt-4">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm font-medium">{t("connectors.sqlParameters")}</p>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={parameters.fields.length >= 32}
            onClick={() => parameters.append({ name: "", type: "text" })}
          >
            <Plus className="size-4" aria-hidden />
            {t("connectors.addSQLParameter")}
          </Button>
        </div>
        {parameters.fields.map((parameter, parameterIndex) => (
          <ParameterFields
            key={parameter.id}
            form={form}
            templateIndex={index}
            parameterIndex={parameterIndex}
            onRemove={() => parameters.remove(parameterIndex)}
          />
        ))}
        {parameters.fields.length === 0 && (
          <p className="text-xs text-muted-foreground">
            {t("connectors.noSQLParameters")}
          </p>
        )}
      </div>
    </div>
  );
}

type NumberFieldName =
  | `templates.${number}.timeoutSeconds`
  | `templates.${number}.maxRows`
  | `templates.${number}.maxBytes`;

function TemplateNumberField({
  form,
  name,
  label,
  min,
  max,
}: {
  form: UseFormReturn<SQLInstanceValues>;
  name: NumberFieldName;
  label: string;
  min: number;
  max: number;
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
              type="number"
              min={min}
              max={max}
              value={field.value}
              onBlur={field.onBlur}
              onChange={(event) => field.onChange(event.target.valueAsNumber)}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function ParameterFields({
  form,
  templateIndex,
  parameterIndex,
  onRemove,
}: {
  form: UseFormReturn<SQLInstanceValues>;
  templateIndex: number;
  parameterIndex: number;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_10rem_2.5rem] items-start gap-2">
      <FormField
        control={form.control}
        name={`templates.${templateIndex}.parameters.${parameterIndex}.name`}
        render={({ field }) => (
          <FormItem>
            <FormControl>
              <Input
                {...field}
                className="font-mono"
                autoComplete="off"
                aria-label={t("connectors.sqlParameterName", { number: parameterIndex + 1 })}
                placeholder={t("connectors.sqlParameterNamePlaceholder")}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={`templates.${templateIndex}.parameters.${parameterIndex}.type`}
        render={({ field }) => (
          <FormItem>
            <Select value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger aria-label={t("connectors.sqlParameterType", { number: parameterIndex + 1 })}>
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                {(["text", "integer", "number", "boolean", "timestamp"] as const).map((type) => (
                  <SelectItem key={type} value={type}>
                    {t(`connectors.sqlParameterTypes.${type}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />
      <Button
        type="button"
        size="icon"
        variant="ghost"
        onClick={onRemove}
        aria-label={t("connectors.removeSQLParameter", { number: parameterIndex + 1 })}
      >
        <Trash2 className="size-4" aria-hidden />
      </Button>
    </div>
  );
}
