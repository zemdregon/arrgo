import { buildTraceHeaders, finishTrace, startClientTrace, TraceAttributes } from "./observability/tracing";

export type TokenResolver = () => string | undefined;

export interface HttpClientOptions {
  baseUrl: string;
  tokenProvider?: TokenResolver;
  defaultHeaders?: HeadersInit;
  traceAttributes?: TraceAttributes;
}

export interface HttpRequestOptions<T> {
  baseUrl?: string;
  traceName?: string;
  traceAttributes?: TraceAttributes;
  parse?: (response: Response) => Promise<T>;
}

function normalizeBaseUrl(baseUrl: string): string {
  if (!baseUrl) {
    throw new Error("Base URL is required");
  }
  return baseUrl.replace(/\/$/, "");
}

export class HttpClient {
  private readonly baseUrl: string;

  private readonly tokenProvider?: TokenResolver;

  private readonly defaultHeaders?: HeadersInit;

  private readonly traceAttributes: TraceAttributes;

  constructor(options: HttpClientOptions) {
    this.baseUrl = normalizeBaseUrl(options.baseUrl);
    this.tokenProvider = options.tokenProvider;
    this.defaultHeaders = options.defaultHeaders;
    this.traceAttributes = options.traceAttributes ?? {};
  }

  private resolveBaseUrl(override?: string): string {
    if (!override) {
      return this.baseUrl;
    }
    return normalizeBaseUrl(override);
  }

  private mergeHeaders(input?: HeadersInit): Headers {
    const headers = new Headers();
    if (this.defaultHeaders) {
      new Headers(this.defaultHeaders).forEach((value, key) => {
        headers.set(key, value);
      });
    }
    if (input) {
      new Headers(input).forEach((value, key) => {
        headers.set(key, value);
      });
    }
    return headers;
  }

  private resolveToken(): string | undefined {
    try {
      const token = this.tokenProvider?.();
      return token?.trim() || undefined;
    } catch (error) {
      return undefined;
    }
  }

  private async extractErrorMessage(response: Response, url: string): Promise<string> {
    const base = `Request to ${url} failed with status ${response.status}`;
    const contentType = response.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
      try {
        const payload = (await response.json()) as { error?: string; message?: string };
        const message = payload.error?.trim() || payload.message?.trim();
        if (message) {
          return message;
        }
      } catch (parseError) {
        const message =
          parseError instanceof Error ? parseError.message : String(parseError);
        return `${base}: ${message}`;
      }
    } else {
      try {
        const text = (await response.text()).trim();
        if (text) {
          return `${base}: ${text}`;
        }
      } catch (readError) {
        const message = readError instanceof Error ? readError.message : String(readError);
        return `${base}: ${message}`;
      }
    }
    return base;
  }

  async request<T>(path: string, init: RequestInit, options: HttpRequestOptions<T> = {}): Promise<T> {
    const baseUrl = this.resolveBaseUrl(options.baseUrl);
    const url = `${baseUrl}${path}`;
    const method = (init.method ?? "GET").toUpperCase();
    const trace = startClientTrace(
      options.traceName ?? `HTTP ${method} ${path}`,
      {
        transport: "rest",
        url,
        method,
        ...this.traceAttributes,
        ...options.traceAttributes,
      },
    );

    const headers = this.mergeHeaders(init.headers);
    if (!headers.has("Accept")) {
      headers.set("Accept", "application/json");
    }
    const token = this.resolveToken();
    if (token && !headers.has("Authorization")) {
      headers.set("Authorization", `Bearer ${token}`);
    }
    const traceHeaders = buildTraceHeaders(trace);
    for (const [key, value] of Object.entries(traceHeaders)) {
      if (!headers.has(key)) {
        headers.set(key, value);
      }
    }

    let response: Response;
    try {
      response = await fetch(url, {
        ...init,
        headers,
        cache: init.cache ?? "no-store",
      });
    } catch (error) {
      finishTrace(trace, "error", { url, method }, error);
      const message = error instanceof Error ? error.message : String(error);
      throw new Error(`Request to ${url} failed: ${message}`);
    }

    if (!response.ok) {
      const message = await this.extractErrorMessage(response, url);
      finishTrace(trace, "error", { url, method, httpStatus: response.status }, message);
      throw new Error(message);
    }

    const parser = options.parse ?? (async (payload: Response) => (await payload.json()) as T);
    try {
      const data = await parser(response);
      finishTrace(trace, "success", { url, method, httpStatus: response.status });
      return data;
    } catch (error) {
      finishTrace(trace, "error", { url, method, httpStatus: response.status }, error);
      const message =
        error instanceof Error ? error.message : `Failed to parse response: ${String(error)}`;
      throw new Error(message);
    }
  }

  async get<T>(path: string, options?: HttpRequestOptions<T>): Promise<T> {
    return this.request<T>(
      path,
      {
        method: "GET",
      },
      options,
    );
  }

  async post<T>(path: string, body: unknown, options?: HttpRequestOptions<T>): Promise<T> {
    return this.request<T>(
      path,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body ?? {}),
      },
      options,
    );
  }

  async put<T>(path: string, body: unknown, options?: HttpRequestOptions<T>): Promise<T> {
    return this.request<T>(
      path,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body ?? {}),
      },
      options,
    );
  }
}
