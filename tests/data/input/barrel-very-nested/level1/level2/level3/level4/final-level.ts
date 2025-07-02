export const DEEPLY_NESTED_CONSTANT = "This is deeply nested!";

export class DeepestClass {
  private depth = 4;
  
  getDepth(): number {
    return this.depth;
  }
  
  getMessage(): string {
    return `I'm at depth ${this.depth}`;
  }
}

export function deepestFunction(input: string): string {
  return `Processed at deepest level: ${input}`;
}
