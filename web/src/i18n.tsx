import * as React from "react";
import type { Dictionary } from "./locales/zh";
import { en as enDict } from "./locales/en";
import { zh as zhDict } from "./locales/zh";
import { readStored, writeStored } from "./lib/utils";

export type Lang = "zh" | "en";

export const LANG_STORAGE_KEY = "fp-lang";

/** First visit: follow navigator.language (zh* → 中文，其余英文). */
export function detectLang(): Lang {
  const saved = readStored(LANG_STORAGE_KEY);
  if (saved === "zh" || saved === "en") return saved;
  try {
    return /^zh\b|^zh-/i.test(window.navigator?.language || "") ? "zh" : "en";
  } catch {
    return "en";
  }
}

export type Vars = Record<string, string | number>;

function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
}

/** Builds the t() for one language; zh is the source dictionary, en falls back to zh for missing keys. */
export function makeT(lang: Lang) {
  return (key: string, vars?: Vars): string => {
    const dict: Dictionary = lang === "zh" ? zhDict : enDict;
    const raw = dict[key] ?? zhDict[key] ?? key;
    return interpolate(raw, vars);
  };
}

export type T = ReturnType<typeof makeT>;

type Ctx = {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: T;
};

const I18nContext = React.createContext<Ctx | null>(null);

/** Exposed for class components (ErrorBoundary) and tests that need the raw context. */
export { I18nContext };

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [lang, setLangState] = React.useState<Lang>(detectLang);

  React.useEffect(() => {
    document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
  }, [lang]);

  const setLang = React.useCallback((next: Lang) => {
    setLangState(next);
    writeStored(LANG_STORAGE_KEY, next);
  }, []);

  const value = React.useMemo<Ctx>(() => ({ lang, setLang, t: makeT(lang) }), [lang, setLang]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): Ctx {
  const ctx = React.useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used inside <I18nProvider>");
  return ctx;
}

/** Translation function only. */
export function useT(): T {
  return useI18n().t;
}

export function useLang(): Lang {
  return useI18n().lang;
}
