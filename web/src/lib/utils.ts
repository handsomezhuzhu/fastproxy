import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

const numFmt = new Intl.NumberFormat("en", { maximumFractionDigits: 1 });

/**
 * Storage accessor that never touches the bare `localStorage` global:
 * Node ≥22 exposes a `localStorage` that warns ("--localstorage-file") on access, which the
 * jsdom smoke test turns into a console.error. Always go through `window`.
 */
export function storage(): Storage | null {
  try {
    return window.localStorage ?? null;
  } catch {
    return null;
  }
}

export function readStored(key: string): string | null {
  try {
    return storage()?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

export function writeStored(key: string, value: string) {
  try {
    storage()?.setItem(key, value);
  } catch {
    /* storage unavailable */
  }
}


/** Human-readable byte size, e.g. 1.2 MB */
export function bytes(n: number): string {
  n = Number(n) || 0;
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let i = -1;
  do {
    n /= 1024;
    i++;
  } while (n >= 1024 && i < units.length - 1);
  return `${numFmt.format(n)} ${units[i]}`;
}

function intlLocale(locale?: string) {
  return locale ? (locale.toLowerCase().startsWith("zh") ? "zh-CN" : "en") : "zh-CN";
}

/**
 * Relative time such as “3 分钟前” / “3 minutes ago”.
 * `locale` should be the active i18n language ("zh" / "en"); defaults to zh for back-compat.
 */
export function timeAgo(iso: string, locale = "zh"): string {
  const t = Date.parse(iso);
  if (!t) return iso;
  const lc = intlLocale(locale);
  const relFmt = new Intl.RelativeTimeFormat(lc, { numeric: "auto" });
  const seconds = Math.round((t - Date.now()) / 1000);
  if (Math.abs(seconds) < 60) return relFmt.format(seconds, "second");
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return relFmt.format(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return relFmt.format(hours, "hour");
  return new Intl.DateTimeFormat(lc, { hour12: false }).format(new Date(t));
}

/** HH:mm:ss for log rows, in the active language's locale. */
export function clock(iso: string, locale = "zh"): string {
  const t = Date.parse(iso);
  const fmt = new Intl.DateTimeFormat(intlLocale(locale), {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
  return t ? fmt.format(new Date(t)) : iso;
}

/** Absolute date-time (local timezone) for tooltips and cards. */
export function dateTime(seconds: number, locale = "zh"): string {
  if (!seconds) return "–";
  return new Intl.DateTimeFormat(intlLocale(locale), {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(seconds * 1000));
}
