export interface FeatureAConfig {
  apiUrl: string;
  timeout: number;
  retries: number;
}

export type FeatureAResponse<T = unknown> = {
  success: boolean;
  data: T;
  error?: string;
};
