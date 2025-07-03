import { ApiError } from "@shared";

export interface PaginatedResponse<T> {
  data: T[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    hasNext: boolean;
    hasPrev: boolean;
  };
}

export interface ApiRequest<T = unknown> {
  method: "GET" | "POST" | "PUT" | "DELETE";
  url: string;
  headers?: Record<string, string>;
  body?: T;
  params?: Record<string, string>;
}

export interface ApiResponse<T = unknown> {
  status: number;
  data: T;
  error?: ApiError;
  headers: Record<string, string>;
}

export type ApiEndpoint = 
  | { path: "/users"; method: "GET"; response: PaginatedResponse<unknown> }
  | { path: "/users/:id"; method: "GET"; response: unknown }
  | { path: "/users"; method: "POST"; body: unknown; response: unknown };
