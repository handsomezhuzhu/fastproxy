// Fetch the live demo's real API payloads into a fixture for jsdom testing.
const BASE = process.argv[2] || "https://7788-agqzhfpx7x7bf6naipbsjpah4u.instabox.tech";
const PASS = process.argv[3];
if (!PASS) {
  console.error("usage: node scripts/fetch-fixture.mjs <base-url> <password>");
  process.exit(2);
}

let cookie = "";
async function call(path, init = {}) {
  const res = await fetch(BASE + path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(cookie ? { cookie } : {}),
      ...(init.headers || {}),
    },
  });
  const setCookie = res.headers.get("set-cookie");
  if (setCookie) cookie = setCookie.split(";")[0];
  const text = await res.text();
  let json = null;
  try {
    json = JSON.parse(text);
  } catch {
    /* not json */
  }
  return { status: res.status, json };
}

const login = await call("/api/login", { method: "POST", body: JSON.stringify({ password: PASS }) });
if (login.status !== 200) {
  console.error("login failed", login.status);
  process.exit(1);
}
const data = await call("/api/data");
const logs = await call("/api/logs?limit=50&level=debug");
const fixture = {
  // v2：port_range 由后端提供；老后端没有则补默认值，保证 fixture 与 UI 类型一致。
  data: data.json ? { port_range: { min: 10001, max: 10050 }, ...data.json } : data.json,
  logs: logs.json,
};
const fs = await import("node:fs");
fs.writeFileSync("scripts/fixture.json", JSON.stringify(fixture, null, 2));
console.log("fixture saved: data=", data.status, "logs=", logs.status);
console.log("ports:", data.json?.ports?.length, "accounts:", data.json?.accounts?.length, "upstreams:", data.json?.upstreams?.length);
