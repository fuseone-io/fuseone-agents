import type { ReactNode } from "react";
import { Panel } from "@/components/shared/panel";

/** One of an account's acts: what it is, what it means, and the means to do it. */
export function ServiceAccountSection({
  title,
  explains,
  action,
  children,
}: {
  title: string;
  explains: string;
  action?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <Panel title={title} action={action}>
      <div className="flex flex-col gap-3">
        <p className="text-xs text-muted-foreground">{explains}</p>
        {children}
      </div>
    </Panel>
  );
}
