// Complex type definitions using multiple imports
import { BaseEntity, User } from "@shared/shared-interfaces";

export interface AppState {
  user: User | null;
  isLoading: boolean;
  error: string | null;
  features: {
    featureA: boolean;
    featureB: boolean;
  };
}

export interface AppConfig extends BaseEntity {
  theme: "light" | "dark";
  language: string;
  notifications: {
    email: boolean;
    push: boolean;
    sms: boolean;
  };
}

export type AppEvent = 
  | { type: "USER_LOGIN"; payload: User }
  | { type: "USER_LOGOUT" }
  | { type: "FEATURE_TOGGLE"; payload: { feature: string; enabled: boolean } }
  | { type: "ERROR"; payload: string };

export interface AppContextValue {
  state: AppState;
  dispatch: (event: AppEvent) => void;
  config: AppConfig;
}
