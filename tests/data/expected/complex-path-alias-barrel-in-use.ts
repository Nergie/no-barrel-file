// Complex import scenario using multiple path aliases and barrel files
import { FeatureAService } from "@feature-a/feature-a-service";
import { FeatureAConfig } from "@feature-a/feature-a-types";
import { FeatureBComponent } from "@feature-b/nested-folder-1/nested-folder-2/nested-folder-3/feature-b-component";
import { featureBUtils } from "@feature-b/nested-folder-1/nested-folder-2/feature-b-utils";
import { APP_NAME, API_VERSION, FEATURE_FLAGS } from "@shared/shared-constants";
import { User, UserRole, BaseEntity } from "@shared/shared-interfaces";
import { Logger, LogLevel } from "@utils/logger";
import { HttpClient } from "@utils/http-client";
import { Button, Modal } from "~components/ui";
import { Form, FormField } from "~components/forms";
import { AppState, AppConfig, AppEvent } from "~types/app-types";
import { PaginatedResponse, ApiRequest } from "~types/api-types";

// Usage examples demonstrating complex interactions
class Application {
  private logger: Logger;
  private httpClient: HttpClient;
  private featureAService: FeatureAService;

  constructor() {
    this.logger = new Logger(LogLevel.INFO);
    this.httpClient = new HttpClient({ baseUrl: "https://api.example.com" });
    
    const featureAConfig: FeatureAConfig = {
      apiUrl: "https://feature-a.example.com",
      timeout: 5000,
      retries: 3
    };
    this.featureAService = new FeatureAService(featureAConfig);
    
    this.logger.info(`Initializing ${APP_NAME} version ${API_VERSION}`);
  }

  async initializeFeatures(): Promise<void> {
    if (FEATURE_FLAGS.ENABLE_FEATURE_A) {
      this.logger.info("Feature A is enabled");
      await this.featureAService.fetchData("test-endpoint");
    }

    if (FEATURE_FLAGS.ENABLE_FEATURE_B) {
      this.logger.info("Feature B is enabled");
      const component = new FeatureBComponent({
        title: "Feature B Component",
        data: { test: "data" }
      });
      component.render();
    }
  }

  createUserForm(): Form {
    const fields: FormField[] = [
      {
        name: "name",
        type: "text",
        required: true,
        validation: (value) => value.length >= 2
      },
      {
        name: "email", 
        type: "email",
        required: true,
        validation: (value) => value.includes("@")
      }
    ];

    return new Form(fields);
  }

  createUser(formData: Form): User {
    const userData = formData.toUser();
    const user: User = {
      id: featureBUtils.generateId(),
      createdAt: new Date(),
      updatedAt: new Date(),
      name: userData.name!,
      email: userData.email!,
      role: UserRole.USER
    };

    this.logger.info("Created user:", user);
    return user;
  }

  getAppState(): AppState {
    return {
      user: null,
      isLoading: false,
      error: null,
      features: {
        featureA: FEATURE_FLAGS.ENABLE_FEATURE_A,
        featureB: FEATURE_FLAGS.ENABLE_FEATURE_B
      }
    };
  }
}

// Example usage
const app = new Application();
const form = app.createUserForm();
form.setFieldValue("name", "John Doe");
form.setFieldValue("email", "john@example.com");

if (form.validate()) {
  const user = app.createUser(form);
  console.log("User created successfully:", user);
}

export { Application };
