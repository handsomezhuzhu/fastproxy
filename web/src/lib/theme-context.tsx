import * as React from "react";
import { useColorMode, type ResolvedTheme, type ThemeMode } from "./theme";

type ThemeContextValue = {
  mode: ThemeMode;
  resolved: ResolvedTheme;
  setMode: (mode: ThemeMode) => void;
  toggle: () => void;
};

const ThemeContext = React.createContext<ThemeContextValue | null>(null);

/** Shares one theme state between the shell and the unauthenticated login page. */
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const value = useColorMode();
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const value = React.useContext(ThemeContext);
  if (!value) throw new Error("useTheme must be used inside ThemeProvider");
  return value;
}
