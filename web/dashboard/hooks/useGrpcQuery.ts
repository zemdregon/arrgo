"use client";

import { useMemo } from "react";
import useSWR, { Key, SWRConfiguration, SWRResponse } from "swr";
import {
  createGrpcLibraryClient,
  GrpcLibraryClient,
  GrpcLibraryClientOptions,
} from "../lib/api";
import { useEnvironment } from "../components/environment/EnvironmentContext";

type ClientFactory = (options: GrpcLibraryClientOptions) => GrpcLibraryClient;

export interface UseGrpcQueryOptions<Data = unknown, Err = Error>
  extends Omit<SWRConfiguration<Data, Err>, "isPaused"> {
  baseUrl?: string;
  enabled?: boolean;
  clientFactory?: ClientFactory;
}

export function useGrpcQuery<Data = unknown, Err = Error>(
  key: Key,
  queryFn: (client: GrpcLibraryClient) => Promise<Data>,
  options: UseGrpcQueryOptions<Data, Err> = {},
): SWRResponse<Data, Err> {
  const { current } = useEnvironment();
  const { baseUrl: overrideBaseUrl, enabled = true, clientFactory, ...swrOptions } = options;
  const baseUrl = overrideBaseUrl ?? current.apiUrl;

  const client = useMemo(() => {
    const factory = clientFactory ?? createGrpcLibraryClient;
    return factory({ baseUrl });
  }, [baseUrl, clientFactory]);

  const resolvedKey = useMemo(() => {
    if (!enabled) {
      return null;
    }
    if (typeof key === "function") {
      return (...args: unknown[]) => {
        const resolved = (key as (...params: unknown[]) => Key)(...args);
        if (resolved == null) {
          return resolved;
        }
        if (Array.isArray(resolved)) {
          return [...resolved, baseUrl];
        }
        return [resolved, baseUrl];
      };
    }
    if (key == null) {
      return null;
    }
    if (Array.isArray(key)) {
      return [...key, baseUrl];
    }
    return [key, baseUrl];
  }, [enabled, key, baseUrl]);

  return useSWR<Data, Err>(resolvedKey, () => queryFn(client), swrOptions);
}
