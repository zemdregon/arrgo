"use client";

import { useMemo } from "react";
import { useEnvironment } from "../components/environment/EnvironmentContext";
import { RestApiClient, RestApiClientOptions } from "../lib/api";

export type UseRestClientOptions = RestApiClientOptions;

export function useRestClient(options: UseRestClientOptions = {}): RestApiClient {
  const { current } = useEnvironment();
  const baseUrl = options.baseUrl ?? current.apiUrl;
  const tokenProvider = options.tokenProvider;

  return useMemo(() => {
    return new RestApiClient({ baseUrl, tokenProvider });
  }, [baseUrl, tokenProvider]);
}
