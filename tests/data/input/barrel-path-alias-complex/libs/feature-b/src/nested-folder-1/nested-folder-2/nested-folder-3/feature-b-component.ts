// Import from feature-a using path alias - this creates a cross-dependency
import { FeatureAResponse } from "@feature-a";

export interface FeatureBProps {
  title: string;
  data: unknown;
  onUpdate?: (data: unknown) => void;
}

export class FeatureBComponent {
  private props: FeatureBProps;

  constructor(props: FeatureBProps) {
    this.props = props;
  }

  render(): string {
    return `<div>${this.props.title}</div>`;
  }
  
  handleResponse<T>(response: FeatureAResponse<T>): void {
    if (response.success && this.props.onUpdate) {
      this.props.onUpdate(response.data);
    }
  }
}
