"use client";

import { PropsWithChildren, useMemo, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SWRConfig } from "swr";
import { ThemeProvider } from "../theme/ThemeProvider";
import { EnvironmentProvider } from "../environment/EnvironmentContext";
import { DiagnosticsProvider } from "../diagnostics/DiagnosticsProvider";
import { ErrorBoundary } from "../diagnostics/ErrorBoundary";
import { AuthProvider } from "../auth/AuthContext";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        refetchOnWindowFocus: false,
        retry: 1,
      },
      mutations: {
        retry: 1,
      },
    },
  });
}

function createSWRProvider() {
  return new Map();
}

export function AppProviders({ children }: PropsWithChildren) {
  const [queryClient] = useState(createQueryClient);
  const swrValue = useMemo(
    () => ({
      provider: createSWRProvider,
      dedupingInterval: 2000,
      focusThrottleInterval: 2000,
      errorRetryCount: 1,
      revalidateOnFocus: false,
    }),
    [],
  );

  return (
    <QueryClientProvider client={queryClient}>
      <SWRConfig value={swrValue}>
        <ThemeProvider>
          <EnvironmentProvider>
            <DiagnosticsProvider>
              <ErrorBoundary>
                <AuthProvider>{children}</AuthProvider>
              </ErrorBoundary>
            </DiagnosticsProvider>
          </EnvironmentProvider>
        </ThemeProvider>
      </SWRConfig>
    </QueryClientProvider>
  );
}
