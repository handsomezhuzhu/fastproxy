import * as React from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { I18nContext } from "@/i18n";
import type { T } from "@/i18n";

type Props = { children: React.ReactNode };
type State = { error: Error | null };

/** Catches render errors and shows a visible message instead of a black page. */
export class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null };
  // 类组件：直接读 i18n context，避免为翻译包一层函数组件。
  static contextType = I18nContext;
  declare context: React.ContextType<typeof I18nContext>;

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error("render error:", error, info);
  }

  render() {
    const t: T = this.context?.t ?? ((k: string) => k);
    if (!this.state.error) return this.props.children;
    return (
      <div className="flex min-h-[50vh] items-center justify-center p-6">
        <Card className="w-full max-w-lg">
          <CardContent className="flex flex-col gap-3 p-6">
            <h2 className="text-base font-semibold text-destructive">{t("shell.renderError")}</h2>
            <p className="text-xs text-muted-foreground">{t("shell.renderErrorDesc")}</p>
            <pre className="max-h-40 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap break-all">
              {this.state.error.message}
              {"\n"}
              {this.state.error.stack}
            </pre>
            <div className="flex gap-2">
              <Button onClick={() => window.location.reload()}>{t("shell.reload")}</Button>
              <Button
                variant="outline"
                onClick={() => {
                  this.setState({ error: null });
                  window.location.hash = "#/dashboard";
                }}
              >
                {t("shell.backToDashboard")}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }
}
