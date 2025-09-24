"use client";

import { QueryKey, useQuery, UseQueryOptions, UseQueryResult } from "@tanstack/react-query";
import { RestApiClient, RestApiClientOptions } from "../lib/api";
import { useRestClient } from "./useRestClient";

type RestClientConfig = Pick<RestApiClientOptions, "baseUrl" | "tokenProvider">;

export interface UseRestQueryOptions<
  TQueryFnData,
  TError,
  TData,
  TQueryKey extends QueryKey,
> extends Omit<UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>, "queryKey" | "queryFn">,
    Partial<RestClientConfig> {}

export function useRestQuery<
  TQueryFnData = unknown,
  TError = Error,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>(
  queryKey: TQueryKey,
  queryFn: (client: RestApiClient) => Promise<TQueryFnData>,
  options?: UseRestQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
): UseQueryResult<TData, TError> {
  const { baseUrl, tokenProvider, ...queryOptions } = options ?? {};
  const restClient = useRestClient({ baseUrl, tokenProvider });

  return useQuery({
    queryKey,
    queryFn: () => queryFn(restClient),
    ...(queryOptions as Omit<
      UseRestQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
      keyof RestClientConfig
    >),
  });
}
