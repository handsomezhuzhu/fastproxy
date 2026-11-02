import React from "react";
import ReactDOM from "react-dom/client";
import { ThemeProvider } from "next-themes";
import "@fontsource-variable/geist";
import App from "./App";
import { THEME_STORAGE_KEY } from "./lib/theme";
import "./index.css";

// 暗亮主题的真身是 src/lib/theme.ts（三态：跟随系统 / 亮 / 暗，首屏由 index.html 内联脚本处理）。
// 这里保留 next-themes 只因为 shadcn 的 sonner 封装内部调用 useTheme()，需要 provider 存在：
//   - storageKey 与 lib/theme 保持一致；
//   - 不设 attribute/enableSystem，避免 next-themes 自己再写一遍 <html> class 与 localStorage。
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider storageKey={THEME_STORAGE_KEY} defaultTheme="system">
      <App />
    </ThemeProvider>
  </React.StrictMode>
);
