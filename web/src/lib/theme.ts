import * as React from "react";

import { readStored, writeStored } from "./utils";

/**
 * 主题模式：system（跟随系统）/ light / dark。
 *
 * 与 next-themes 的约定保持一致（localStorage key = `fp-theme`，值 `system` | `light` | `dark`），
 * 这样 shadcn 的 sonner 封装（内部用 next-themes 的 useTheme）也能读到同一份状态。
 * 首次访问（没有 localStorage）必须是跟随系统；后续手动选择才持久化。
 */
export type ThemeMode = "system" | "light" | "dark";
export type ResolvedTheme = "light" | "dark";

export const THEME_STORAGE_KEY = "fp-theme";

export function readThemeMode(): ThemeMode {
  // 显式走 window.localStorage（见 lib/utils storage()）：jsdom 冒烟环境下裸 localStorage
  // 会触发 Node 实验性 --localstorage-file 告警并被当成 console.error。
  const v = readStored(THEME_STORAGE_KEY);
  if (v === "light" || v === "dark" || v === "system") return v;
  return "system";
}

/** index.html 里的内联脚本用同一套逻辑写入 data-theme，避免首屏闪白。 */
export function systemTheme(): ResolvedTheme {
  try {
    return window.matchMedia?.("(prefers-color-scheme: dark)")?.matches ? "dark" : "light";
  } catch {
    return "light";
  }
}

export function resolveTheme(mode: ThemeMode): ResolvedTheme {
  return mode === "system" ? systemTheme() : mode;
}

/** Applies the resolved theme to <html> (class + color-scheme + data-theme + theme-color). */
export function applyTheme(resolved: ResolvedTheme) {
  const root = document.documentElement;
  root.classList.toggle("dark", resolved === "dark");
  root.style.colorScheme = resolved;
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute("content", resolved === "dark" ? "#171717" : "#ffffff");
}

export function useColorMode() {
  const [mode, setModeState] = React.useState<ThemeMode>(readThemeMode);
  const [resolved, setResolved] = React.useState<ResolvedTheme>(() => resolveTheme(mode));

  React.useEffect(() => {
    applyTheme(resolved);
  }, [resolved]);

  // 跟随系统时监听 prefers-color-scheme，实时切换。
  React.useEffect(() => {
    if (mode !== "system") return;
    const mq = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (!mq) return;
    const onChange = () => setResolved(mq.matches ? "dark" : "light");
    if (mq.addEventListener) {
      mq.addEventListener("change", onChange);
      return () => mq.removeEventListener("change", onChange);
    }
    // 旧版 Safari
    mq.addListener?.(onChange);
    return () => mq.removeListener?.(onChange);
  }, [mode]);

  const setMode = React.useCallback((next: ThemeMode) => {
    setModeState(next);
    setResolved(resolveTheme(next));
    // system 也写进去：表示用户显式选过“跟随系统”，与“从未设置”区分开但行为一致。
    writeStored(THEME_STORAGE_KEY, next);
  }, []);

  // 头部按钮：在亮 ⇄ 暗之间切换（从 system 出发时切到另一种模式）。
  const toggle = React.useCallback(() => {
    setMode(resolved === "dark" ? "light" : "dark");
  }, [resolved, setMode]);

  return { mode, resolved, setMode, toggle };
}
