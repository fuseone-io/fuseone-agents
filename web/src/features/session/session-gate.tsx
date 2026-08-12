import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Skeleton } from "@/components/ui/skeleton";
import { ErrorState } from "@/components/shared/states";
import { useMe, sessionKeys } from "@/features/session/api";
import { useSignInOptions, providerKeys } from "@/features/session/providers";
import { SetupPage } from "@/features/session/setup-page";
import { SignInPage } from "@/features/session/sign-in-page";

/**
 * Decides which of the three things a visitor can be looking at: an
 * installation nobody has claimed, a console they are not signed in to, or the
 * console.
 *
 * The order matters. Setup is offered only while nobody holds Curator
 * anywhere, so the screen that can create an administrator disappears the
 * moment one exists.
 */
export function SessionGate({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const options = useSignInOptions();
  const me = useMe();

  if (options.isLoading || me.isLoading) return <Loading />;

  if (options.error) {
    return (
      <div className="p-10">
        <ErrorState
          error={options.error}
          onRetry={() => void options.refetch()}
        />
      </div>
    );
  }

  // An installation with nothing to sign in to renders the console. The
  // server has already warned its operator that every caller has full access.
  if (options.data && !options.data.authRequired) return <>{children}</>;

  if (options.data?.bootstrapPending) {
    return (
      <SetupPage
        onClaimed={() => {
          void queryClient.invalidateQueries({ queryKey: sessionKeys.me });
          void queryClient.invalidateQueries({ queryKey: providerKeys.all });
        }}
      />
    );
  }

  if (!me.data) return <SignInPage providers={options.data?.providers ?? []} />;

  return <>{children}</>;
}

function Loading() {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-svh items-center justify-center bg-sidebar p-4">
      <Skeleton className="h-64 w-full max-w-md rounded-2xl" />
      <span className="sr-only">{t("common.loading")}</span>
    </div>
  );
}
