import { Component, type ReactNode, type ErrorInfo } from "react";
import { AlertTriangle, RotateCcw } from "lucide-react";
import { Button } from "../ui/button";
import { Card } from "../ui/card";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  onReset?: () => void;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  override componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    console.error("[ErrorBoundary]", error, errorInfo);
  }

  handleReset = (): void => {
    this.setState({ hasError: false, error: null });
    this.props.onReset?.();
  };

  override render(): ReactNode {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <Card className="w-full max-w-xl space-y-5">
          <div className="flex items-center gap-3">
            <div className="rounded-2xl bg-[color:var(--error-bg)] p-3 text-error">
              <AlertTriangle className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-xl font-semibold tracking-tight text-foreground">Component Error</h2>
              <p className="text-sm text-secondary">
                A UI component crashed. The rest of the page is still functional.
              </p>
            </div>
          </div>
          <div className="rounded-2xl border border-border bg-background p-4 font-mono text-sm text-secondary">
            {this.state.error?.message ?? "Unknown error"}
          </div>
          <div className="flex flex-wrap gap-3">
            <Button type="button" onClick={this.handleReset}>
              <RotateCcw className="mr-2 h-4 w-4" />
              Retry
            </Button>
          </div>
        </Card>
      );
    }

    return this.props.children;
  }
}