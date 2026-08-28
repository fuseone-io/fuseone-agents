import { Check, ChevronsUpDown, ShieldAlert } from "lucide-react";
import { useState } from "react";
import { useWatch, type UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";
import { useEvidenceRuns } from "@/features/runs/api";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

/**
 * Which run this was learned from.
 *
 * Chosen from finished runs in the selected scope. The artifact and the digest
 * are still the ledger's to answer; this picker only names which execution the
 * resolver must prove.
 */
export function EvidenceFields({
  form,
}: {
  form: UseFormReturn<MemoryFormValues>;
}) {
  const company = useWatch({ control: form.control, name: "company" });
  const area = useWatch({ control: form.control, name: "area" });
  const scope = `${company}/${area}`;

  return (
    <FormField
      control={form.control}
      name="evidenceRunId"
      render={({ field }) => (
        <EvidenceRunPicker
          key={scope}
          company={company}
          area={area}
          value={field.value}
          onChange={field.onChange}
        />
      )}
    />
  );
}

function EvidenceRunPicker({
  company,
  area,
  value,
  onChange,
}: {
  company: string;
  area: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const runs = useEvidenceRuns({ company, area, search, enabled: open });
  const unavailable = !company || !area;

  function pick(runID: string) {
    onChange(runID);
    setSearch("");
    setOpen(false);
  }

  return (
    <FormItem>
      <FormLabel>{t("memory.evidenceRun")}</FormLabel>
      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          if (!next) setSearch("");
        }}
      >
        <PopoverTrigger asChild>
          <FormControl>
            <Button
              type="button"
              variant="outline"
              role="combobox"
              aria-expanded={open}
              disabled={unavailable}
              className="w-full justify-between font-normal"
            >
              <span
                className={cn(
                  "min-w-0 truncate font-mono",
                  !value && "font-sans text-muted-foreground",
                )}
              >
                {value || t("memory.chooseEvidenceRun")}
              </span>
              <ChevronsUpDown aria-hidden className="size-4 shrink-0 opacity-50" />
            </Button>
          </FormControl>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-(--radix-popover-trigger-width) p-0"
        >
          <Command shouldFilter={false}>
            <CommandInput
              value={search}
              onValueChange={setSearch}
              placeholder={t("memory.searchEvidenceRuns")}
            />
            <CommandList>
              {(runs.isLoading || runs.isSettling) && (
                <p className="py-6 text-center text-sm text-muted-foreground">
                  {t("memory.loadingEvidenceRuns")}
                </p>
              )}
              {!runs.isLoading && !runs.isSettling && (
                <CommandEmpty>
                  {runs.isError
                    ? t("memory.evidenceRunsUnavailable")
                    : t("memory.noEvidenceRuns")}
                </CommandEmpty>
              )}
              {!runs.isSettling && runs.items.length > 0 && (
                <CommandGroup
                  heading={
                    search.trim()
                      ? t("memory.runSearchResults")
                      : t("memory.recentRuns")
                  }
                >
                  {runs.items.map((run) => (
                    <CommandItem
                      key={run.runId}
                      value={run.runId}
                      onSelect={() => pick(run.runId)}
                      className="items-start py-2"
                    >
                      <Check
                        aria-hidden
                        className={cn(
                          "mt-0.5 size-4",
                          value === run.runId ? "opacity-100" : "opacity-0",
                        )}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="flex min-w-0 items-center gap-2">
                          <Mono className="min-w-0 flex-1 truncate">
                            {run.runId}
                          </Mono>
                          <span className="shrink-0 text-2xs text-muted-foreground">
                            {formatRelative(run.startedAt)}
                          </span>
                        </span>
                        <span className="mt-1 flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                          <span className="min-w-0 truncate">
                            {run.agentId || t("memory.runWithoutAgent")}
                          </span>
                          {run.labels?.includes("untrusted") && (
                            <Badge className="h-5 bg-warning-surface text-warning">
                              <ShieldAlert aria-hidden />
                              {t("memory.outsideData")}
                            </Badge>
                          )}
                        </span>
                      </span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}
              {!runs.isSettling && runs.hasMore && (
                <p className="border-t px-3 py-2 text-xs text-muted-foreground">
                  {t("memory.moreEvidenceRuns")}
                </p>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      <FormDescription>{t("memory.evidenceHint")}</FormDescription>
    </FormItem>
  );
}
