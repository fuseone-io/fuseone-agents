export const serviceAccountKeys = {
  all: ["service-accounts"] as const,
  tokens: (account: string) => ["service-accounts", account, "tokens"] as const,
};
