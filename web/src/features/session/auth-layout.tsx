import type { ReactNode } from "react";
import { BrandLogoWordmark } from "@/features/branding/branding-provider";

/**
 * The shell for the screens that exist before there is a session: no sidebar,
 * no navigation, nothing that would suggest the console is available yet.
 */
export function AuthLayout({
  icon,
  title,
  description,
  children,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-svh items-center justify-center bg-sidebar p-4">
      <div className="flex w-full max-w-md flex-col gap-6 rounded-2xl border bg-background p-8 shadow-sm">
        <div className="flex flex-col gap-3">
          <BrandLogoWordmark />
          <div className="flex items-start gap-2">
            <span className="mt-0.5">{icon}</span>
            <div>
              <h1 className="text-xl font-medium tracking-tight">{title}</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                {description}
              </p>
            </div>
          </div>
        </div>
        {children}
      </div>
    </div>
  );
}
