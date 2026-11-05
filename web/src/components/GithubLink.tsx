import { buttonVariants } from "@/components/ui/button";
import { useI18n } from "@/i18n";
import { cn } from "@/lib/utils";

export const GITHUB_REPO_URL = "https://github.com/handsomezhuzhu/fastproxy";

/** Top-right GitHub repository icon link, matching ThemeSwitch / AuthLanguageToggle height (h-7). */
export function GithubLink() {
  const { t } = useI18n();
  const label = t("shell.github");
  return (
    <a
      href={GITHUB_REPO_URL}
      target="_blank"
      rel="noreferrer noopener"
      aria-label={label}
      title={label}
      className={cn(
        buttonVariants({ variant: "outline", size: "icon-sm" }),
        "text-muted-foreground hover:text-foreground [&_svg]:size-3.5"
      )}
    >
      <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
        <path d="M12 1.5C6.201 1.5 1.5 6.201 1.5 12c0 4.639 3.008 8.574 7.181 9.963.525.097.717-.228.717-.506 0-.25-.01-1.077-.014-1.954-2.921.635-3.537-1.24-3.537-1.24-.478-1.213-1.166-1.536-1.166-1.536-.953-.652.072-.639.072-.639 1.054.074 1.609 1.082 1.609 1.082.937 1.605 2.458 1.141 3.058.873.095-.679.366-1.142.666-1.404-2.332-.265-4.784-1.166-4.784-5.19 0-1.146.41-2.084 1.081-2.819-.109-.265-.468-1.333.102-2.779 0 0 .882-.282 2.888 1.077a10.06 10.06 0 0 1 2.627-.353c.892.004 1.79.12 2.629.353 2.004-1.359 2.885-1.077 2.885-1.077.572 1.446.213 2.514.104 2.779.673.735 1.08 1.673 1.08 2.819 0 4.034-2.456 4.922-4.795 5.182.377.325.713.966.713 1.947 0 1.406-.013 2.539-.013 2.885 0 .28.19.608.722.505C19.496 20.57 22.5 16.637 22.5 12c0-5.799-4.701-10.5-10.5-10.5Z" />
      </svg>
    </a>
  );
}
