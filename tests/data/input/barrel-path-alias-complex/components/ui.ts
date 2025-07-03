export interface ButtonProps {
  text: string;
  onClick: () => void;
  variant?: "primary" | "secondary";
}

export class Button {
  private props: ButtonProps;

  constructor(props: ButtonProps) {
    this.props = props;
  }

  render(): string {
    const className = `btn btn-${this.props.variant || "primary"}`;
    return `<button class="${className}">${this.props.text}</button>`;
  }
}

export class Modal {
  private isOpen = false;

  open(): void {
    this.isOpen = true;
  }

  close(): void {
    this.isOpen = false;
  }

  render(content: string): string {
    if (!this.isOpen) return "";
    return `<div class="modal">${content}</div>`;
  }
}
