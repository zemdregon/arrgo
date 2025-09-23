import { DependencyList, useCallback, useEffect, useState } from "react";

interface AsyncState<T> {
  data?: T;
  loading: boolean;
  error?: string;
}

export function useApiData<T>(
  loader: () => Promise<T>,
  deps: DependencyList = []
): AsyncState<T> & { refresh: () => void } {
  const [state, setState] = useState<AsyncState<T>>({ loading: true });

  const refresh = useCallback(() => {
    setState((previous) => ({ ...previous, loading: true, error: undefined }));
    loader()
      .then((result) => {
        setState({ data: result, loading: false });
      })
      .catch((error) => {
        const message = error instanceof Error ? error.message : String(error);
        setState({ error: message, loading: false });
      });
  }, [loader, ...deps]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { ...state, refresh };
}
