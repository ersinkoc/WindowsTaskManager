import { useEffect } from "react";
import { isRouteErrorResponse, useNavigate } from "react-router";
import { AlertTriangle, RotateCcw } from "lucide-react";
import { Button } from "../ui/button";
import { Card } from "../ui/card";

interface ErrorPageProps {
  error: unknown;
}

export function GlobalErrorHandler() {
  useEffect(() => {
    const handler = (event: ErrorEvent) => {
      console.error("[GlobalError]", event.error);
    };

    const rejectionHandler = (event: PromiseRejectionEvent) => {
      console.error("[UnhandledRejection]", event.reason);
    };

    window.addEventListener("error", handler);
    window.addEventListener("unhandledrejection", rejectionHandler);

    return () => {
      window.removeEventListener("error", handler);
      window.removeEventListener("unhandledrejection", rejectionHandler);
    };
  }, []);

  return null;
}

export function ErrorPage({ error }: ErrorPageProps) {
  const navigate = useNavigate();

  const message = isRouteErrorResponse(error)
    ? `${error.status} ${error.statusText}`
    : error instanceof Error
      ? error.message
      : "Something went wrong";

  return (
    <div className="app-shell page-padding flex min-h-screen items-center justify-center py-10">
      <Card className="w-full max-w-xl space-y-5">
        <div className="flex items-center gap-3">
          <div className="rounded-2xl bg-[color:var(--error-bg)] p-3 text-error">
            <AlertTriangle className="h-5 w-5" />
          </div>
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">Something went wrong</h1>
            <p className="text-sm text-secondary">WTM hit an unexpected error and recovered into a safe fallback.</p>
          </div>
        </div>
        <div className="rounded-2xl border border-border bg-background p-4 font-mono text-sm text-secondary">
          {message}
        </div>
        <div className="flex flex-wrap gap-3">
          <Button type="button" onClick={() => navigate(0)}>
            <RotateCcw className="mr-2 h-4 w-4" />
            Retry
          </Button>
          <Button type="button" variant="secondary" onClick={() => navigate("/")}>
            Back to overview
          </Button>
        </div>
      </Card>
    </div>
  );
}