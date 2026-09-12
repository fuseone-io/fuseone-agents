export function ApprovalDetailRow({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-baseline gap-3">
      <dt className="w-20 shrink-0 text-xs text-muted-foreground">{label}</dt>
      <dd
        className={
          mono
            ? "min-w-0 flex-1 truncate font-mono text-xs"
            : "min-w-0 flex-1 truncate"
        }
      >
        {value}
      </dd>
    </div>
  );
}
