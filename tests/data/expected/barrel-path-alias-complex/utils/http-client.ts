// Import from shared library using path alias
import { ApiError } from "@shared/shared-interfaces";
import { DEFAULT_TIMEOUT } from "@shared/shared-constants";

export interface HttpClientOptions {
  baseUrl?: string;
  timeout?: number;
  headers?: Record<string, string>;
}

export class HttpClient {
  private options: HttpClientOptions;

  constructor(options: HttpClientOptions = {}) {
    this.options = {
      timeout: DEFAULT_TIMEOUT,
      ...options,
    };
  }

  async get<T>(url: string): Promise<T> {
    // Mock implementation
    throw new Error("Mock HTTP client");
  }

  async post<T>(url: string, data: unknown): Promise<T> {
    // Mock implementation 
    throw new Error("Mock HTTP client");
  }

  private handleError(error: unknown): ApiError {
    if (error instanceof Error) {
      return {
        code: "HTTP_ERROR",
        message: error.message,
      };
    }
    return {
      code: "UNKNOWN_ERROR", 
      message: "An unknown error occurred",
    };
  }
}
