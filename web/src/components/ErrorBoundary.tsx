import * as React from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

type Props = { children: React.ReactNode };
type State = { error: Error | null };

/** Catches render errors and shows a visible message instead of a black page. */
export class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error("渲染错误:", error, info);
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="flex min-h-[50vh] items-center justify-center p-6">
        <Card className="w-full max-w-lg">
          <CardContent className="space-y-3 p-6">
            <h2 className="text-base font-semibold text-destructive">页面渲染出错</h2>
            <p className="text-xs text-muted-foreground">
              这是前端运行时错误，不是网络问题。请点击下方按钮重新加载；若持续出现，请把下面这段信息发给维护者。
            </p>
            <pre className="max-h-40 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap break-all">
              {this.state.error.message}
              {"\n"}
              {this.state.error.stack}
            </pre>
            <div className="flex gap-2">
              <Button onClick={() => window.location.reload()}>重新加载</Button>
              <Button
                variant="outline"
                onClick={() => {
                  this.setState({ error: null });
                  window.location.hash = "#/dashboard";
                }}
              >
                返回仪表盘
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }
}
