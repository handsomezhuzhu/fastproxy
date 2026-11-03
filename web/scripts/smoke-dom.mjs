// DOM smoke test: loads the built SPA under jsdom with a stubbed API and
// reports runtime errors (used to catch "black screen after login").
import { JSDOM } from "jsdom";
import { pathToFileURL } from "node:url";
import fs from "node:fs";
import path from "node:path";

const distDir = path.resolve("../admin/dist/assets");
const bundle = fs.readdirSync(distDir).find((f) => /^index-.*\.js$/.test(f));
if (!bundle) {
  console.error("bundle not found in", distDir);
  process.exit(2);
}

const dom = new JSDOM(
  `<!doctype html><html class="dark"><head></head><body><div id="root"></div></body></html>`,
  { url: "http://localhost:7788/", pretendToBeVisual: true }
);
const { window } = dom;
// 固定中文环境：i18n 首次按 navigator.language 判断；同时预置 localStorage，
// 保证断言文案稳定（英文用户首次自然得到 en，e2e 覆盖切换）。
try {
  window.localStorage.setItem("fp-lang", "zh");
} catch {
  /* storage unavailable */
}
Object.defineProperty(window.navigator, "language", { value: "zh-CN", configurable: true });

const defineGlobal = (k, v) =>
  Object.defineProperty(globalThis, k, { value: v, configurable: true, writable: true });

defineGlobal("window", window);
defineGlobal("document", window.document);
defineGlobal("navigator", window.navigator);
defineGlobal("location", window.location);
defineGlobal("HTMLElement", window.HTMLElement);
defineGlobal("Node", window.Node);
defineGlobal("Element", window.Element);
defineGlobal("Event", window.Event);
defineGlobal("CustomEvent", window.CustomEvent);
defineGlobal("MutationObserver", window.MutationObserver);
defineGlobal("getComputedStyle", window.getComputedStyle);
defineGlobal("requestAnimationFrame", (cb) => setTimeout(() => cb(Date.now()), 0));
defineGlobal("cancelAnimationFrame", (id) => clearTimeout(id));
defineGlobal("scrollTo", () => {});
window.scrollTo = () => {};
window.matchMedia = () => ({
  matches: false,
  addEventListener() {},
  removeEventListener() {},
  addListener() {},
  removeListener() {},
});
class RO {
  observe() {}
  unobserve() {}
  disconnect() {}
}
defineGlobal("ResizeObserver", RO);
defineGlobal("IntersectionObserver", RO);
window.ResizeObserver = RO;
window.IntersectionObserver = RO;

const settings = {
  access_log: true,
  log_level: "info",
  log_buffer: 1000,
  log_retention_days: 7,
  dial_timeout_sec: 15,
  handshake_timeout_sec: 60,
  probe_enabled: true,
  probe_interval_sec: 300,
  probe_timeout_sec: 8,
  probe_concurrency: 6,
  probe_target: "api.ipify.org",
};

/** v2：/api/data 带 port_range（容器 PORT_RANGE），新建端口必须在该范围内。 */
const portRange = { min: 10001, max: 10050 };

let data, logEntries;
const fixturePath = path.resolve("scripts/fixture.json");
if (fs.existsSync(fixturePath)) {
  const fx = JSON.parse(fs.readFileSync(fixturePath, "utf8"));
  data = fx.data;
  logEntries = fx.logs?.entries ?? [];
  console.log("using live fixture: upstreams =", data?.upstreams, "ports =", data?.ports?.length);
}
const now = Math.floor(Date.now() / 1000);
const sampleData = {
  version: "v0.4.0",
  settings,
  port_range: portRange,
  upstreams: [
    {
      id: "u1",
      name: "美国节点",
      type: "http",
      host: "45.3.48.250:3129",
      username: "kcl0gwtbpbux",
      password: "secret",
      health: "ok",
      latency_ms: 148,
      exit_ip: "45.3.48.250",
      last_check_at: now,
    },
  ],
  ports: [{ id: "p1", port: 10201, default_upstream: "u1", enabled: true, remark: "主线" }],
  accounts: [
    {
      id: "a1",
      username: "demo",
      password: "demo123",
      port_id: "p1",
      upstream_id: "",
      enabled: true,
      remark: "",
    },
  ],
  status: {
    ports: [
      {
        id: "p1",
        port: 10201,
        enabled: true,
        listening: true,
        conns_total: 5,
        conns_cur: 1,
        bytes_in: 1024,
        bytes_out: 2048,
      },
    ],
    accounts: [
      {
        id: "a1",
        username: "demo",
        port_id: "p1",
        upstream_id: "",
        enabled: true,
        conns_total: 5,
        conns_cur: 1,
        bytes_in: 1024,
        bytes_out: 2048,
      },
    ],
  },
  probe: { server_ip: "198.200.49.224", last_check_at: now },
};

const sampleLogEntries = [
  {
    time: new Date().toISOString(),
    level: "info",
    port: 10201,
    account: "demo",
    client: "1.2.3.4",
    proto: "http",
    target: "api.ipify.org:443",
    status: "ok",
    bytes_in: 100,
    bytes_out: 200,
    duration_ms: 120,
    msg: "",
  },
];

if (!data) {
  data = sampleData;
  logEntries = sampleLogEntries;
}

function jsonBody(obj) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve(obj),
    text: () => Promise.resolve(JSON.stringify(obj)),
  });
}

async function fakeFetch(url) {
  const u = String(url);
  if (u.includes("/api/session")) return jsonBody({ auth: true, version: "v0.4.0" });
  if (u.includes("/api/data")) return jsonBody(data);
  if (u.includes("/api/stats")) return jsonBody({ status: data.status });
  if (u.includes("/api/logs")) return jsonBody({ entries: logEntries });
  // 后端尚未合并 /api/singbox/status：模拟 404，仪表盘状态卡必须优雅降级。
  if (u.includes("/api/singbox/status"))
    return Promise.resolve({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ error: "not found" }),
      text: () => Promise.resolve("not found"),
    });
  return jsonBody({ ok: true });
}
defineGlobal("fetch", fakeFetch);
window.fetch = fakeFetch;

const errors = [];
window.addEventListener("error", (e) =>
  errors.push("window.error: " + (e.error?.stack || e.message))
);
window.addEventListener("unhandledrejection", (e) =>
  errors.push("unhandledrejection: " + (e.reason?.stack || e.reason))
);
const originalError = console.error;
console.error = (...args) => {
  errors.push(
    "console.error: " + args.map((a) => (a instanceof Error ? a.stack : String(a))).join(" ")
  );
  originalError(...args);
};

try {
  await import(pathToFileURL(path.join(distDir, bundle)).href);
} catch (e) {
  console.log("IMPORT FAILED:", e?.stack || e);
  process.exit(1);
}
await new Promise((r) => setTimeout(r, 1000));

const root = window.document.getElementById("root");
const html = root.innerHTML;

// Node may emit its own --localstorage-file warning through console.error; it
// is not an app runtime error and must not fail the black-screen guard.
const runtimeErrors = errors.filter(
  (e) => !e.includes("localstorage-file") && !e.includes("trace-warnings")
);
if (runtimeErrors.length) {
  console.log("=== RUNTIME ERRORS ===");
  for (const e of runtimeErrors) console.log(e, "\n---");
}
console.log("root children:", root.childElementCount);
console.log("html length:", html.length);
console.log("has sidebar:", html.includes("FastProxy"));
console.log("has dashboard:", html.includes("仪表盘"));
// v2 断言：跟随系统主题（jsdom matchMedia=false → light）、无旧接口字段。
console.log("theme follows system:", window.document.documentElement.classList.contains("dark") === false);
console.log("no stale allow_direct UI:", !html.includes("允许直连"));
if (!html.length) {
  console.log("DOM IS EMPTY (black screen reproduced)");
  process.exit(1);
}

// sing-box 已从仪表盘提升为独立导航页：切到 #/singbox 再检查 404 降级文案。
window.location.hash = "#/singbox";
window.dispatchEvent(new window.Event("hashchange"));
await new Promise((r) => setTimeout(r, 800));
const singboxHtml = root.innerHTML;
console.log("has singbox nav:", html.includes("内置 sing-box"));
console.log("singbox unavailable fallback:", singboxHtml.includes("后端暂未提供状态"));

const z = html.includes("仪表盘");
const s = singboxHtml.includes("后端暂未提供状态");
if (!z || !s) {
  console.log("EXPECTED MARKERS MISSING:", { dashboardTitle: z, singboxFallback: s });
  process.exit(1);
}
process.exit(runtimeErrors.length ? 1 : 0);
