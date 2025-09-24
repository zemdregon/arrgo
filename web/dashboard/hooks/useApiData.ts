import { DependencyList, useCallback, useEffect, useState } from "react";

interface AsyncState<T> {
  data?: T;
  loading: boolean;
  error?: string;
}

interface UseApiDataOptions {
  enabled?: boolean;
}

export function useApiData<T>(
  loader: () => Promise<T>,
  deps: DependencyList = [],
  options: UseApiDataOptions = {}
): AsyncState<T> & { refresh: () => void } {
  const enabled = options.enabled ?? true;
  const [state, setState] = useState<AsyncState<T>>({ loading: enabled });

  const refresh = useCallback(() => {
    if (!enabled) {
      setState((previous) => ({ ...previous, loading: false }));
      return;
    }
    setState((previous) => ({ ...previous, loading: true, error: undefined }));
    loader()
      .then((result) => {
        setState({ data: result, loading: false });
      })
      .catch((error) => {
        const message = error instanceof Error ? error.message : String(error);
        setState({ error: message, loading: false });
      });
  }, [enabled, loader, ...deps]);

  useEffect(() => {
    if (!enabled) {
      setState((previous) => ({ ...previous, loading: false }));
      return;
    }
    refresh();
  }, [enabled, refresh]);

  return { ...state, refresh };
}
