// Import types from different path aliases
import { User } from "@shared";

export interface FormField {
  name: string;
  type: "text" | "email" | "password" | "number";
  required?: boolean;
  validation?: (value: string) => boolean;
}

export class Form {
  private fields: FormField[];
  private data: Record<string, string> = {};

  constructor(fields: FormField[]) {
    this.fields = fields;
  }

  setFieldValue(name: string, value: string): void {
    this.data[name] = value;
  }

  getFieldValue(name: string): string {
    return this.data[name] || "";
  }

  validate(): boolean {
    return this.fields.every(field => {
      const value = this.data[field.name];
      if (field.required && !value) return false;
      if (field.validation && !field.validation(value)) return false;
      return true;
    });
  }

  toUser(): Partial<User> {
    return {
      name: this.data.name,
      email: this.data.email,
    };
  }
}
