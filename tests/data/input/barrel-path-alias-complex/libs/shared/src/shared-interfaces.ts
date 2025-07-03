export interface BaseEntity {
  id: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface User extends BaseEntity {
  name: string;
  email: string;
  role: UserRole;
}

export enum UserRole {
  ADMIN = "admin",
  USER = "user",
  GUEST = "guest",
}

export interface ApiError {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}
