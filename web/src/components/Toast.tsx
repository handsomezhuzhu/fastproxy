import * as React from "react";
import { CheckCircle2, XCircle } from "lucide-react";
import { cn } from "../lib/utils";

type ToastItem = { id: number; msg: string; kind: "success" | "error" };
type Listener = (t: ToastItem) => void;

let listeners: Listener[] = [];
let seq = 1;

export function toast(msg: string, kind: "success" | "error" = "success") {
  const item = { id: seq++, msg, kind };
  listeners.forEach((l) => l(item));
}

export function ToastHost() {
  const [items, setItems] = React.useState<ToastItem[]>([]);

  React.useEffect(() => {
    const listener: Listener = (t) => {
      setItems((prev) => [...prev, t]);
      window.setTimeout(() => {
        setItems((prev) => prev.filter((x) => x.id !== t.id));
      }, 3200);
    };
    listeners.push(listener);
    return () => {
      listeners = listeners.filter((l) => l !== listener);
    };
  }, []);

  return (
    <div className="pointer-events-none fixed bottom-6 right-6 z-[100] flex w-80 flex-col gap-2">
      {items.map((t) => (
        <div
          key={t.id}
          className={cn(
            "pointer-events-auto flex items-start gap-2 rounded-md border bg-card px-4 py-3 text-sm shadow-xl",
            t.kind === "error" ? "border-destructive/60" : "border-emerald-500/40"
          )}
        >
          {t.kind === "error" ? (
            <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
          ) : (
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
          )}
          <span className="break-all">{t.msg}</span>
        </div>
      ))}
    </div>
  );
}
