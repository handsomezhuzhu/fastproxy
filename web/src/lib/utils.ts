import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

const numFmt = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 });

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

const relFmt = new Intl.RelativeTimeFormat("zh-CN", { numeric: "auto" });

/** Relative time such as “3 分钟前” (localized, Intl-based). */
export function timeAgo(iso: string): string {
  const t = Date.parse(iso);
  if (!t) return iso;
  const seconds = Math.round((t - Date.now()) / 1000);
  if (Math.abs(seconds) < 60) return relFmt.format(seconds, "second");
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return relFmt.format(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return relFmt.format(hours, "hour");
  return new Intl.DateTimeFormat("zh-CN", { hour12: false }).format(new Date(t));
}

const clockFmt = new Intl.DateTimeFormat("zh-CN", {
  hour12: false,
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
});

/** HH:mm:ss for log rows. */
export function clock(iso: string): string {
  const t = Date.parse(iso);
  return t ? clockFmt.format(new Date(t)) : iso;
}
