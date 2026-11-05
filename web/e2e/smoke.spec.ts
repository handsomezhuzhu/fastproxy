import { expect, test, type Locator, type Page } from "@playwright/test";

/**
 * Browser smoke suite against a real FastProxy backend.
 *
 * Guards the regressions we actually shipped:
 *  - React error #31 / black screen after login (page must render <h1>);
 *  - "上游代理" list empty even though the API returned items
 *    (`GET /api/upstreams` returns `{items,total}`, the UI used `entries`);
 *  - Select popup growing wider than its trigger;
 *  - port create/save crashes on object-rendered payloads.
 *
 * v2 additions (see the UI PR):
 *  - i18n: switching to EN re-renders nav/page titles in English and persists;
 *  - theme: first visit follows the system, the header toggle persists the choice;
 *  - embedded sing-box status card: degrades to "后端暂未提供状态" when
 *    `/api/singbox/status` is not deployed yet (404), instead of crashing;
 *  - unified upstream check (`POST /api/upstreams/check`): tests skip while 404.
 *
 * Data: the suite creates port `10021` (inside the default PORT_RANGE 10001-10050)
 * and `#e2e-*` upstreams through the UI and removes them again. CI starts with a
 * fresh DB file every run, and the cleanup below also makes reruns against the
 * same DB work.
 */

const PORT = 10021;
const UPSTREAM_1 = "http://u:p@1.1.1.1:8080#e2e-1";
const UPSTREAM_2 = "socks5://2.2.2.2:1080#e2e-2";
// VLESS REALITY + XTLS Vision：需要后端用 sing-box 出站导入（旧后端会拒绝）。
const UPSTREAM_REALITY =
  "vless://b831381d-6324-4d53-ad4f-8cda48b30811@3.3.3.3:443" +
  "?security=reality&pbk=H5M42pBqRHTcRX3_LPHDXTeO0cEz_G_zYud220O96As&sid=0123456789abcdef&flow=xtls-rprx-vision&type=tcp&sni=www.apple.com#e2e-reality";
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? "";

const NAV: { id: string; label: string; title: string; en: string }[] = [
  { id: "ports", label: "端口线路", title: "端口线路", en: "Ports" },
  { id: "accounts", label: "账户", title: "账户", en: "Accounts" },
  { id: "upstreams", label: "上游代理", title: "上游代理", en: "Upstreams" },
  { id: "singbox", label: "内置 sing-box", title: "内置 sing-box", en: "Embedded sing-box" },
  { id: "logs", label: "连接日志", title: "连接日志", en: "Connection logs" },
  { id: "settings", label: "系统设置", title: "系统设置", en: "Settings" },
  { id: "dashboard", label: "仪表盘", title: "仪表盘", en: "Dashboard" },
];

test.skip(!ADMIN_PASSWORD, "ADMIN_PASSWORD 未设置：需要真实后端密码才能跑 e2e");

/** React #31 / crashed tree guard: no error boundary text, no page errors. */
function watchRuntime(page: Page) {
  const failures: string[] = [];
  page.on("pageerror", (e) => failures.push(`pageerror: ${e.message}`));
  return failures;
}

async function expectNoCrash(page: Page, failures: string[]) {
  // 文案本地化后中英文都要覆盖。
  await expect(page.getByText(/页面渲染出错|Something went wrong while rendering/)).toHaveCount(0);
  await expect(page.getByText("Minified React error")).toHaveCount(0);
  expect(failures, "浏览器运行时错误").toEqual([]);
}

async function login(page: Page) {
  await page.goto("/");
  const pw = page.getByLabel(/管理密码|Admin password/);
  await expect(pw).toBeVisible();
  await pw.fill(ADMIN_PASSWORD);
  await page.getByRole("button", { name: /^(登录|Sign in)$/ }).click();
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible({ timeout: 20_000 });
}

/**
 * Probe a POST endpoint through the page's own session cookies.
 * Used to skip v2-only assertions while the backend half has not merged yet.
 */
async function postStatus(page: Page, path: string, body: unknown): Promise<number> {
  const res = await page.request.post(path, { data: body });
  return res.status();
}

/** True when the backend lacks the v2 unified check endpoint (404/405). */
async function checkEndpointMissing(page: Page): Promise<boolean> {
  const status = await postStatus(page, "/api/upstreams/check", { ids: [] }).catch(() => 404);
  return status === 404 || status === 405 || status === 501;
}

// 桌面侧栏（移动端 Sheet 会在 DOM 里复制同一批按钮，必须限定 aside）
const nav = (page: Page, label: string) =>
  page.locator("aside").getByRole("button", { name: label, exact: true });

async function goTo(page: Page, id: "dashboard" | "ports" | "accounts" | "upstreams" | "logs" | "settings") {
  await page.goto(`/#/${id}`);
  await page.waitForLoadState("networkidle");
}

/** Open the row action menu (last cell) and run an action by its visible label. */
async function rowAction(row: Locator, label: string) {
  await row.getByRole("button", { name: /操作菜单|操作$|actions$/i }).click();
  const item = row.page().getByRole("menuitem", { name: label, exact: true }).first();
  // Base UI renders the menu through a portal on the next frame; wait for it
  // instead of counting immediately (the old code raced the animation).
  await item.waitFor({ state: "visible", timeout: 5000 }).catch(() => {});
  if (!(await item.isVisible().catch(() => false))) return false;
  await item.click({ force: true });
  return true;
}

async function deletePortRow(page: Page, port: number) {
  const rows = page.locator("tbody tr").filter({ hasText: String(port) });
  if ((await rows.count()) === 0) return;
  if (!(await rowAction(rows.first(), "删除"))) return;
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "删除", exact: true }).click();
  await expect(dialog).toBeHidden();
}

async function deleteUpstreamByName(page: Page, name: string) {
  // Cleanup through the API: menu clicks are covered by dedicated UI tests and
  // the Base UI portal makes cleanup clicks flaky in headless mode.
  try {
    const res = await page.request.get("/api/upstreams?limit=1000&offset=0");
    if (!res.ok()) return false;
    const body = await res.json();
    const item = (body.items ?? []).find((u: { id: string; name: string }) => u.name === name);
    if (!item) return false;
    const del = await page.request.delete(`/api/upstreams/${item.id}`);
    if (!del.ok()) return false;
    // The UI is not notified of API-side cleanup; refresh so callers can assert
    // the row is gone (and afterEach leaves a consistent page).
    await page.reload({ waitUntil: "domcontentloaded" }).catch(() => {});
    return true;
  } catch {
    return false;
  }
}

/** 主题取决于 localStorage；每个用例开始前清掉，避免上一个用例的英文/暗色泄漏。 */
test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    try {
      window.localStorage.clear();
    } catch {
      /* ignore */
    }
  });
});

/** Upstreams currently returned by the list endpoint (or empty on any error). */
async function listUpstreams(page: Page): Promise<{ id: string; name: string }[]> {
  try {
    const res = await page.request.get("/api/upstreams?limit=1000&offset=0");
    if (!res.ok()) return [];
    const body = await res.json();
    return body?.items ?? body?.entries ?? [];
  } catch {
    return [];
  }
}

test.describe.configure({ mode: "serial" });

test.afterEach(async ({ page }) => {
  // Best-effort cleanup so reruns against a reused DB stay green.
  // 用会话 cookie 直接调 API：与界面语言/动画/当前页面无关，也不会被 skip 干扰。
  try {
    if (!ADMIN_PASSWORD) return;
    // 确保已登录（前一个用例可能停在登出/英文界面）。
    const session = await page.request.get("/api/session");
    const authed = (await session.json().catch(() => ({})))?.auth;
    if (!authed) {
      await page.request.post("/api/login", { data: { password: ADMIN_PASSWORD } });
    }
    for (const name of ["e2e-1", "e2e-2", "e2e-ss", "e2e-vless", "e2e-reality"]) {
      for (const u of (await listUpstreams(page)).filter((x) => x.name === name)) {
        await page.request.delete(`/api/upstreams/${u.id}`).catch(() => {});
      }
    }
    const data = await page.request.get("/api/data");
    const ports = (await data.json().catch(() => ({})))?.ports ?? [];
    for (const p of ports) {
      if (Number(p?.port) === PORT) await page.request.delete(`/api/ports/${p.id}`).catch(() => {});
    }
  } catch {
    /* cleanup must never mask the real failure */
  }
});

test("登录页与错误密码提示", async ({ page }) => {
  const failures = watchRuntime(page);
  await page.goto("/");
  await expect(page.getByText(/FastProxy 控制台|FastProxy Console/)).toBeVisible();
  await expect(page.getByRole("heading", { name: /FastProxy/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /^(登录|Sign in)$/ })).toBeVisible();

  await page.getByLabel(/管理密码|Admin password/).fill("definitely-wrong-password");
  await page.getByRole("button", { name: /^(登录|Sign in)$/ }).click();
  await expect(page.getByRole("alert")).toContainText(/密码错误|attempts|Too many/i);
  await expectNoCrash(page, failures);
});

test("登录页：管理密码标签足够大，卡片内不再有闪电图标", async ({ page }) => {
  await page.goto("/");
  const label = page.locator('label[for="admin-password"]');
  await expect(label).toBeVisible();
  const size = await label.evaluate((el) => Number.parseFloat(getComputedStyle(el).fontSize));
  expect(size, "登录标签字号").toBeGreaterThanOrEqual(13);

  // 登录卡片里只保留文字标题，不允许再出现品牌图标（主题/语言按钮在卡片外）。
  const card = page.locator('form[novalidate], form').first().locator("xpath=..");
  await expect(card.locator("svg")).toHaveCount(0);
  await expect(page.locator('header a[href="https://github.com/handsomezhuzhu/fastproxy"]')).toBeVisible();
});

test("侧栏：仅品牌/版本/退出，导航字号可读", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  const aside = page.locator("aside");
  await expect(aside.getByText("FastProxy", { exact: true })).toBeVisible();
  await expect(aside.locator('[translate="no"]')).toHaveText(/^v\S+$/);
  // 旧的“管理员 / 在线”卡片已移除。
  await expect(aside.getByText(/管理员|Administrator/)).toHaveCount(0);
  await expect(aside.getByText(/^(在线|Online)$/)).toHaveCount(0);
  await expect(aside.getByRole("button", { name: /退出登录|Sign out/ })).toBeVisible();

  const ports = aside.getByRole("button", { name: "端口线路", exact: true });
  await expect(ports).toBeVisible();
  // 内置 sing-box 是资源区的一级导航，不再挂在左下角。
  await expect(aside.getByRole("button", { name: "内置 sing-box", exact: true })).toBeVisible();
  const navSize = await ports.evaluate((el) => Number.parseFloat(getComputedStyle(el).fontSize));
  expect(navSize, "侧栏导航字号").toBeGreaterThanOrEqual(15);
  const groupSize = await aside
    .getByText("资源", { exact: true })
    .evaluate((el) => Number.parseFloat(getComputedStyle(el).fontSize));
  expect(groupSize, "侧栏分组字号").toBeGreaterThanOrEqual(13);
  await expectNoCrash(page, failures);
});

test("正确密码登录后进入仪表盘", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  await expect(page.getByRole("heading", { level: 1, name: /仪表盘|Dashboard/ })).toBeVisible();
  await expect(page.getByText(/端口线路|Ports/).first()).toBeVisible();
  await expect(page.getByText(/上游健康|Upstream health/)).toBeVisible();
  await expect(page.locator("aside nav")).toBeVisible();
  await expect(page.locator('header a[href="https://github.com/handsomezhuzhu/fastproxy"]')).toBeVisible();
  await expectNoCrash(page, failures);
});

test("七个导航页都能渲染（React #31 兜底）", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  for (const item of NAV) {
    await nav(page, item.label).click();
    await expect(page.getByRole("heading", { level: 1, name: item.title })).toBeVisible();
    await expect(page.locator("main")).not.toBeEmpty();
    await expectNoCrash(page, failures);
  }
});

test("语言切换：英文导航与页面标题出现，并且刷新后保持", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  await expect(page.getByRole("heading", { level: 1, name: "仪表盘" })).toBeVisible();

  // 侧栏底部的 中 / EN 切换
  const sidebarSwitch = page.getByRole("group", { name: /语言|Language/ });
  await expect(sidebarSwitch).toBeVisible();
  await sidebarSwitch.getByRole("button", { name: "EN", exact: true }).click();

  await expect(page.getByRole("heading", { level: 1, name: "Dashboard" })).toBeVisible();
  for (const item of NAV) {
    await expect(nav(page, item.en)).toBeVisible();
  }
  // 面包屑与页头按钮也走 i18n
  await expect(page.getByRole("button", { name: "Copy access" })).toBeVisible();
  await expect(page.getByText(/No connections yet|Recent connections/).first()).toBeVisible();
  await expectNoCrash(page, failures);

  // 其他页面标题同步变英文
  await nav(page, "Upstreams").click();
  await expect(page.getByRole("heading", { level: 1, name: "Upstreams" })).toBeVisible();
  await nav(page, "Settings").click();
  await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Save settings" })).toBeVisible();

  // localStorage 记忆：整页重新加载后仍是英文（waitUntil: "load" 确保没有中止导航）。
  await page.goto(`${new URL(page.url()).origin}/#/settings`, { waitUntil: "load" });
  await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible({ timeout: 20_000 });
  await expect(nav(page, "Dashboard")).toBeVisible();

  // 切回中文
  await page.getByRole("group", { name: /Language/ }).getByRole("button", { name: "中", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "系统设置" })).toBeVisible();
  await expectNoCrash(page, failures);
});

test("主题：三态（跟随系统/亮/暗）切换并持久化", async ({ page }) => {
  const failures = watchRuntime(page);
  await page.emulateMedia({ colorScheme: "dark" });
  await login(page);

  const html = page.locator("html");
  // 首次（无 localStorage）：跟随系统 → 暗色
  await expect(html).toHaveClass(/dark/);
  await expect(html).toHaveAttribute("lang", "zh-CN");
  const group = page.getByRole("group", { name: /外观|Appearance/ });
  await expect(group).toBeVisible();

  // 手动切到亮色 → 持久化 fp-theme=light
  await group.getByRole("button", { name: /浅色|Light/ }).click();
  await expect(html).not.toHaveClass(/dark/);
  expect(await page.evaluate(() => window.localStorage.getItem("fp-theme"))).toBe("light");

  // 系统主题变化不再影响手动选择
  await page.emulateMedia({ colorScheme: "dark" });
  await page.waitForTimeout(200);
  await expect(html).not.toHaveClass(/dark/);
  await page.goto(`${new URL(page.url()).origin}/#/dashboard`, { waitUntil: "load" });
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible({ timeout: 20_000 });
  await expect(html).not.toHaveClass(/dark/);

  // 手动切到暗色 → 持久化
  await page.getByRole("group", { name: /外观|Appearance/ }).getByRole("button", { name: /深色|Dark/ }).click();
  await expect(html).toHaveClass(/dark/);
  expect(await page.evaluate(() => window.localStorage.getItem("fp-theme"))).toBe("dark");

  // 回到跟随系统：系统为暗色，所以页面仍是暗色但存储为 system
  await page.getByRole("group", { name: /外观|Appearance/ }).getByRole("button", { name: /跟随系统|Follow system/ }).click();
  await expect(html).toHaveClass(/dark/);
  expect(await page.evaluate(() => window.localStorage.getItem("fp-theme"))).toBe("system");

  await expectNoCrash(page, failures);
});

test("内置 sing-box 页面：状态、刷新与详情", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  // 和账户/上游代理同级的一级导航。
  await nav(page, "内置 sing-box").click();
  await expect(page.getByRole("heading", { level: 1, name: "内置 sing-box" })).toBeVisible();

  const panel = page.getByTestId("singbox-panel");
  await expect(panel).toBeVisible();
  await expect(panel.getByText(/运行状态|Runtime status/)).toBeVisible();

  const probe = await page.request.get("/api/singbox/status");
  const refresh = panel.getByRole("button", { name: /刷新 sing-box 状态|Refresh sing-box status/ });
  await expect(refresh).toBeVisible();

  if (probe.status() === 404 || probe.status() === 405) {
    await expect(panel.getByText(/后端暂未提供状态|Status not provided by the backend/)).toBeVisible();
    await expect(panel.getByText(/PORT_RANGE|sing-box\/status|upgrade the backend|升级后端/i).first()).toBeVisible();
  } else {
    await expect(panel.getByText(/运行中|未运行|Running|Stopped/).first()).toBeVisible({ timeout: 15_000 });
    await expect(panel.getByText(/已注册上游|Registered upstreams/)).toBeVisible();
    await expect(panel.getByText(/最近重载|Last reload/)).toBeVisible();
  }

  // 刷新按钮不应报错，也不应触发整页崩溃。
  await refresh.click();
  await expect(panel).toBeVisible();
  await expectNoCrash(page, failures);
});

test("上游代理：批量导入后列表能显示 e2e 节点", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  await nav(page, "上游代理").click();
  await expect(page.getByRole("heading", { level: 1, name: "上游代理" })).toBeVisible();

  // 空列表时工具栏不存在，入口在 EmptyState 的按钮里 —— 两处文案一致，直接按名字点。
  await page.getByRole("button", { name: "批量导入" }).click();
  const dialog = page.getByRole("dialog", { name: "批量导入上游" });
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("节点文本").fill(`${UPSTREAM_1}\n${UPSTREAM_2}`);
  await dialog.getByRole("button", { name: "导入文本" }).click();
  await expect(dialog.getByText(/新增 [12]/)).toBeVisible();
  await expect(dialog.getByText("新增 2", { exact: true })).toBeVisible();

  await dialog.getByRole("button", { name: "关闭" }).click();
  await expect(dialog).toBeHidden();

  // 核心回归：入口函数读错字段（entries vs items）时这里是空的。
  await expect(page.getByText("还没有上游代理")).toHaveCount(0);
  await expect(page.getByText("e2e-1", { exact: true })).toBeVisible();
  await expect(page.getByText("e2e-2", { exact: true })).toBeVisible();
  const rows = page.locator("tbody tr").filter({ hasText: /e2e-[12]/ });
  await expect(rows).toHaveCount(2);
  await expectNoCrash(page, failures);

  // 清理：UI 删除两条 e2e 上游（afterEach 也会兜底）。
  await deleteUpstreamByName(page, "e2e-1");
  await deleteUpstreamByName(page, "e2e-2");
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-1" })).toHaveCount(0);
});

test("上游代理：同一批节点重复导入不会新增", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "上游代理").click();
  await expect(page.getByRole("heading", { level: 1, name: "上游代理" })).toBeVisible();

  const importText = async (expectAdded: string) => {
    await page.getByRole("button", { name: "批量导入" }).first().click();
    const dialog = page.getByRole("dialog", { name: "批量导入上游" });
    await expect(dialog).toBeVisible();
    await dialog.getByLabel("节点文本").fill(UPSTREAM_1);
    await dialog.getByRole("button", { name: "导入文本" }).click();
    await expect(dialog.getByText(expectAdded, { exact: true })).toBeVisible();
    return dialog;
  };

  const first = await importText("新增 1");
  await first.getByRole("button", { name: "关闭" }).click();
  await expect(first).toBeHidden();
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-1" })).toHaveCount(1);

  // 第二次导入同一节点：只报告重复，不新增行。
  const second = await importText("新增 0");
  await expect(second.getByText(/重复 1|Duplicates 1/)).toBeVisible();
  await second.getByRole("button", { name: "关闭" }).click();
  await expect(second).toBeHidden();
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-1" })).toHaveCount(1);
  await expectNoCrash(page, failures);
});

test("订阅：相同地址不能重复创建", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "订阅").click();
  await expect(page.getByRole("heading", { level: 1, name: "订阅管理" })).toBeVisible();

  const url = "https://example.invalid/sub?token=e2e-dup";

  // 第一次：正常创建。
  await page.getByRole("button", { name: "添加订阅" }).first().click();
  const first = page.getByRole("dialog", { name: "添加订阅" });
  await expect(first).toBeVisible();
  await first.getByLabel("名称").fill("e2e-sub-dup");
  await first.getByLabel("订阅地址").fill(url);
  await first.getByRole("button", { name: "保存", exact: true }).click();
  await expect(first).toBeHidden();
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-sub-dup" })).toHaveCount(1);

  // 第二次：同一 URL，后端拒绝，表单保留错误。
  await page.getByRole("button", { name: "添加订阅" }).first().click();
  const second = page.getByRole("dialog", { name: "添加订阅" });
  await expect(second).toBeVisible();
  await second.getByLabel("名称").fill("e2e-sub-dup-2");
  await second.getByLabel("订阅地址").fill(url);
  await second.getByRole("button", { name: "保存", exact: true }).click();
  await expect(second.getByRole("alert")).toContainText(/已存在|already exists/);
  await expect(second).toBeVisible();
  await second.getByRole("button", { name: "取消" }).click();
  await expect(second).toBeHidden();
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-sub-dup-2" })).toHaveCount(0);

  // Cleanup through the API.
  const list = await (await page.request.get("/api/subscriptions")).json();
  const sub = (list.items ?? []).find((x: { name: string }) => x.name === "e2e-sub-dup");
  expect(sub).toBeTruthy();
  await page.request.delete(`/api/subscriptions/${sub.id}?delete_nodes=true`);
  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-sub-dup" })).toHaveCount(0);
  await expectNoCrash(page, failures);
});

test("上游代理：检测全部走统一接口（v2 未合并则跳过）", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "上游代理").click();
  await expect(page.getByRole("heading", { level: 1, name: "上游代理" })).toBeVisible();

  await page.getByRole("button", { name: "批量导入" }).click();
  const dialog = page.getByRole("dialog", { name: "批量导入上游" });
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("节点文本").fill(UPSTREAM_1);
  await dialog.getByRole("button", { name: "导入文本" }).click();
  await expect(dialog.getByText("新增 1", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "关闭" }).click();

  // 旧后端没有 POST /api/upstreams/check：跳过（保留断言供联调后启用）。
  if (await checkEndpointMissing(page)) {
    // 旧后端没有 POST /api/upstreams/check：跳过（afterEach 会用 API 清掉 e2e-1）。
    test.skip(true, "后端尚未提供 POST /api/upstreams/check（等 v2 后端合并后启用）");
  }

  const progress = page.getByRole("button", { name: /检测全部|检测中 \d+\/\d+/ });
  await progress.click();
  // 逐行实时刷新：每个 id 一个请求，完成后状态/延迟/失败原因写回该行。
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-1" }).getByText(/正常|失败/)).toBeVisible({
    timeout: 30_000,
  });
  await expect(page.getByRole("button", { name: /^(检测全部|检测中 \d+\/\d+)$/ })).toBeVisible({ timeout: 30_000 });
  await expectNoCrash(page, failures);
});

test("上游代理：表单可新增 Shadowsocks 与 VLESS（并回填编辑）", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "上游代理").click();
  await expect(page.getByRole("heading", { level: 1, name: "上游代理" })).toBeVisible();

  // 空列表时也要能手动添加上游（EmptyState 入口）。
  await page.getByRole("button", { name: "添加上游" }).first().click();
  const form = page.getByRole("dialog", { name: "添加上游代理" });
  await expect(form).toBeVisible();
  await form.getByLabel("名称").fill("e2e-ss");
  await form.getByLabel("协议").click();
  await page.getByRole("option", { name: "Shadowsocks" }).click();
  await form.getByLabel("服务器地址").fill("1.1.1.1:8388");
  await form.getByLabel("加密方式").click();
  await page.getByRole("option", { name: "aes-128-gcm" }).click();
  await form.getByLabel("密码").fill("e2e-secret");
  await form.getByRole("button", { name: "保存", exact: true }).click();
  await expect(form).toBeHidden();

  const ssRow = page.locator("tbody tr").filter({ hasText: "e2e-ss" });
  await expect(ssRow).toHaveCount(1);
  await expect(ssRow.getByText("SS", { exact: true })).toBeVisible();
  await expect(ssRow.getByText("aes-128-gcm")).toBeVisible();

  // VLESS：字段切换 + TLS 开关（关闭后 SNI 隐藏）。v2：开关只改 security，不再发送 tls。
  await page.getByRole("button", { name: "添加上游" }).first().click();
  const form2 = page.getByRole("dialog", { name: "添加上游代理" });
  await expect(form2).toBeVisible();
  await form2.getByLabel("名称").fill("e2e-vless");
  await form2.getByLabel("协议").click();
  await page.getByRole("option", { name: "VLESS" }).click();
  await form2.getByLabel("服务器地址").fill("2.2.2.2:443");
  await form2.getByLabel("UUID").fill("b831381d-6324-4d53-ad4f-8cda48b30811");
  // VLESS：TLS 开关只表达 security（v2 起不再发送 tls 字段）。
  await expect(form2.getByLabel("SNI")).toBeVisible();
  await expect(form2.getByLabel("安全层")).toContainText("TLS");
  await form2.getByRole("switch", { name: "启用 TLS" }).click();
  await expect(form2.getByLabel("SNI")).toHaveCount(0);
  await expect(form2.getByLabel("安全层")).toContainText("无加密");
  // 旧后端（v2 前）只认 tls 字段，security=none 会被回填成 TLS：此时跳过保存断言。
  const v2Backend = !(await checkEndpointMissing(page));
  await form2.getByRole("switch", { name: "启用 TLS" }).click();
  await form2.getByRole("button", { name: "保存", exact: true }).click();
  await expect(form2).toBeHidden();

  const vlessRow = page.locator("tbody tr").filter({ hasText: "e2e-vless" });
  await expect(vlessRow).toHaveCount(1);
  await expect(vlessRow.getByText("VLESS", { exact: true })).toBeVisible();
  if (v2Backend) {
    // security 是唯一事实来源：e2e-vless 保存的是 TLS，副行应显示 TLS。
    await expect(vlessRow.getByText(/· TLS/)).toBeVisible();
  }

  // 编辑回填：按类型回填字段，而不是拼 ss:// 地址。
  expect(await rowAction(ssRow.first(), "编辑"), "行操作菜单里应有「编辑」").toBe(true);
  const edit = page.getByRole("dialog").filter({ hasText: "e2e-ss" });
  await expect(edit).toBeVisible();
  await expect(edit.getByLabel("服务器地址")).toHaveValue("1.1.1.1:8388");
  await expect(edit.getByLabel("密码")).toHaveValue("e2e-secret");
  await edit.getByRole("button", { name: "取消" }).click();
  await expect(edit).toBeHidden();
  await expectNoCrash(page, failures);

  await deleteUpstreamByName(page, "e2e-ss");
  await deleteUpstreamByName(page, "e2e-vless");
  await expect(page.getByText("e2e-ss", { exact: true })).toHaveCount(0);
});

test("上游代理：VLESS REALITY 表单（解析失败优雅提示 + REALITY 字段 + WS/gRPC 切换）", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "上游代理").click();
  await expect(page.getByRole("heading", { level: 1, name: "上游代理" })).toBeVisible();
  // 页面描述必须反映 REALITY 支持（不再有「VMess 仅保存配置」）。
  await expect(page.getByText(/VLESS（含 REALITY）/)).toBeVisible();

  await page.getByRole("button", { name: "添加上游" }).first().click();
  const form = page.getByRole("dialog", { name: "添加上游代理" });
  await expect(form).toBeVisible();

  // 1) 分享链接解析：后端已支持 /api/upstreams/parse，点「解析并填入」后
  //    协议、地址、REALITY 字段、Flow、SNI 都应直接回填。
  await form.getByLabel("名称").fill("e2e-reality");
  await form.getByLabel("协议").click();
  await page.getByRole("option", { name: "VLESS", exact: true }).click();
  await form.getByLabel("粘贴分享链接（可选）").fill(UPSTREAM_REALITY);
  await form.getByRole("button", { name: "解析并填入" }).click();
  await expect(form.getByLabel("服务器地址")).toHaveValue("3.3.3.3:443");
  await expect(form.getByLabel("安全层")).toContainText("REALITY");
  await expect(form.getByLabel("公钥 pbk")).toHaveValue("H5M42pBqRHTcRX3_LPHDXTeO0cEz_G_zYud220O96As");
  await expect(form.getByLabel("Short ID")).toHaveValue("0123456789abcdef");
  await expect(form.getByLabel("SNI")).toHaveValue("www.apple.com");
  await expect(form.getByLabel("Flow")).toContainText("xtls-rprx-vision");
  await expectNoCrash(page, failures);

  // 2) REALITY 字段：选 REALITY 后 pbk/sid/fingerprint 出现，TLS 开关隐藏；SNI 仍可见（REALITY 自带 TLS）。
  await form.getByLabel("服务器地址").fill("3.3.3.3:443");
  await form.getByLabel("UUID").fill("b831381d-6324-4d53-ad4f-8cda48b30811");
  await form.getByLabel("安全层").click();
  await page.getByRole("option", { name: "REALITY", exact: true }).click();
  await expect(form.getByLabel("公钥 pbk")).toBeVisible();
  await expect(form.getByLabel("Short ID")).toBeVisible();
  await expect(form.getByLabel("指纹")).toContainText("chrome");
  await expect(form.getByRole("switch", { name: "启用 TLS" })).toHaveCount(0);
  await expect(form.getByLabel("SNI")).toBeVisible();
  await form.getByLabel("公钥 pbk").fill("x");
  await form.getByLabel("Short ID").fill("y");
  await form.getByLabel("SNI").fill("www.apple.com");

  // Flow = XTLS Vision
  await form.getByLabel("Flow").click();
  await page.getByRole("option", { name: "xtls-rprx-vision" }).click();

  // 3) 传输方式：WS 显示 path/host 头，gRPC 显示 service name，TCP 两者都隐藏。
  await form.getByLabel("传输方式").click();
  await page.getByRole("option", { name: "WebSocket" }).click();
  await expect(form.getByLabel("WS Path")).toBeVisible();
  await expect(form.getByLabel("WS Host 头")).toBeVisible();
  await form.getByLabel("WS Path").fill("/e2e");
  await form.getByLabel("传输方式").click();
  await page.getByRole("option", { name: "gRPC" }).click();
  await expect(form.getByLabel("WS Path")).toHaveCount(0);
  await expect(form.getByLabel("gRPC Service Name")).toBeVisible();
  await form.getByLabel("gRPC Service Name").fill("e2e-grpc");
  await expectNoCrash(page, failures);

  // 4) VMess：不再有「暂未支持」提示，同样是 sing-box 出站。
  await form.getByLabel("协议").click();
  await page.getByRole("option", { name: "VMess", exact: true }).click();
  await expect(form.getByText(/暂未支持/)).toHaveCount(0);
  await expect(form.getByLabel("gRPC Service Name")).toBeVisible();
  await form.getByRole("button", { name: "取消" }).click();
  await expect(form).toBeHidden();
  await expectNoCrash(page, failures);
});

test("上游代理：批量导入 REALITY/Vision 节点后列表显示 e2e-reality", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "上游代理").click();
  await expect(page.getByRole("heading", { level: 1, name: "上游代理" })).toBeVisible();

  await page.getByRole("button", { name: "批量导入" }).click();
  const dialog = page.getByRole("dialog", { name: "批量导入上游" });
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("节点文本").fill(UPSTREAM_REALITY);
  await dialog.getByRole("button", { name: "导入文本" }).click();

  // 后端合并 sing-box 之前，旧导入器会拒绝 REALITY：此时用例跳过（保留断言供联调后启用）。
  const added = dialog.getByText("新增 1", { exact: true });
  const skipped = await dialog
    .getByText(/问题 [1-9]/)
    .isVisible({ timeout: 20_000 })
    .catch(() => false);
  test.skip(skipped, "当前后端仍拒绝 REALITY 导入（等 sing-box 后端合并后启用）");
  await expect(added).toBeVisible();

  await dialog.getByRole("button", { name: "关闭" }).click();
  await expect(dialog).toBeHidden();

  const row = page.locator("tbody tr").filter({ hasText: "e2e-reality" });
  await expect(row).toHaveCount(1);
  await expect(row.getByText("VLESS", { exact: true })).toBeVisible();
  await expect(row.getByText("REALITY", { exact: true })).toBeVisible();
  await expect(row.getByText("Vision", { exact: true })).toBeVisible();
  await expectNoCrash(page, failures);

  await deleteUpstreamByName(page, "e2e-reality");
  await expect(page.getByText("e2e-reality", { exact: true })).toHaveCount(0);
});

test("订阅：创建、列表展示与删除", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "订阅").click();
  await expect(page.getByRole("heading", { level: 1, name: "订阅管理" })).toBeVisible();

  await page.getByRole("button", { name: "添加订阅" }).first().click();
  const dialog = page.getByRole("dialog", { name: "添加订阅" });
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("名称").fill("e2e-sub");
  await dialog.getByLabel("订阅地址").fill("https://example.invalid/sub?token=e2e");
  await dialog.getByLabel("刷新间隔（分钟）").fill("60");
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog).toBeHidden();

  const row = page.locator("tbody tr").filter({ hasText: "e2e-sub" });
  await expect(row).toHaveCount(1);
  await expect(row.getByText("60 分钟")).toBeVisible();

  // Cleanup through the API and refresh; the create handler may still be trying
  // to fetch the fake URL in the background, which must not affect the UI.
  const list = await (await page.request.get("/api/subscriptions")).json();
  const sub = (list.items ?? []).find((x: { name: string }) => x.name === "e2e-sub");
  expect(sub).toBeTruthy();
  await page.request.delete(`/api/subscriptions/${sub.id}?delete_nodes=true`);
  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-sub" })).toHaveCount(0);
  await expectNoCrash(page, failures);
});

test("端口线路：新增/删除 10021 与每页下拉宽度", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "端口线路").click();
  await expect(page.getByRole("heading", { level: 1, name: "端口线路" })).toBeVisible();

  // 1) 新增线路
  await page.getByRole("button", { name: "新增线路" }).first().click();
  const form = page.getByRole("dialog", { name: "新增端口线路" });
  await expect(form).toBeVisible();
  const portInput = form.getByLabel("监听端口");
  // v2：/api/data 的 port_range 决定输入框 min/max 与范围提示（默认 10001-10050）。
  await expect(portInput).toHaveAttribute("min", /^\d+$/);
  await expect(portInput).toHaveAttribute("max", /^\d+$/);
  await expect(form.getByText(/PORT_RANGE/)).toBeVisible();

  await portInput.fill(String(PORT));
  await form.getByLabel("备注").fill("e2e");
  await form.getByRole("button", { name: "保存", exact: true }).click();

  // 保存成功后弹「默认账号已生成」，关掉它；表单弹窗必须已关闭。
  // 注意：两个弹窗会交接，所以必须按名字定位，不能用裸 getByRole("dialog")。
  await expect(form).toBeHidden();
  const created = page.getByRole("dialog", { name: "默认账号已生成" });
  if (await created.isVisible().catch(() => false)) {
    await created.getByRole("button", { name: "知道了" }).click();
    await expect(created).toBeHidden();
  }
  await expect(page.locator("tbody tr").filter({ hasText: String(PORT) })).toHaveCount(1);
  await expectNoCrash(page, failures);

  // 2) 每页下拉：弹层不得比触发器宽（防回归：Select 撑宽）
  const trigger = page.getByRole("combobox", { name: "每页行数" });
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox, "每页触发器应可见").not.toBeNull();
  await trigger.click();
  const popup = page.locator('[data-slot="select-content"]');
  await expect(popup).toBeVisible();
  const popupBox = await popup.boundingBox();
  expect(popupBox).not.toBeNull();
  expect(popupBox!.width, "下拉弹层宽度").toBeLessThanOrEqual(triggerBox!.width + 2);

  await page.getByRole("option", { name: "50 / 页" }).click();
  await expect(popup).toBeHidden();
  await expect(trigger).toContainText("50 / 页");
  await expect(page.locator("table")).toBeVisible();
  await expectNoCrash(page, failures);

  // 3) 删除 10021 清理
  await deletePortRow(page, PORT);
  await expect(page.locator("tbody tr").filter({ hasText: String(PORT) })).toHaveCount(0);
  await expectNoCrash(page, failures);
});

test("端口线路：越界端口在前端被拦截（PORT_RANGE）", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "端口线路").click();
  await page.getByRole("button", { name: "新增线路" }).first().click();
  const form = page.getByRole("dialog", { name: "新增端口线路" });
  await expect(form).toBeVisible();
  const max = Number(await form.getByLabel("监听端口").getAttribute("max"));
  await form.getByLabel("监听端口").fill(String(max + 1));
  await form.getByRole("button", { name: "保存", exact: true }).click();
  // 不提交到后端：直接在表单里给本地化错误，弹窗保持打开。
  await expect(form.getByRole("alert")).toContainText(/PORT_RANGE|范围/);
  await expect(form).toBeVisible();
  await form.getByRole("button", { name: "取消" }).click();
  await expect(form).toBeHidden();
  await expectNoCrash(page, failures);
});

test("账户与复制向导：新建账户并验证复制接入格式切换", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  const accPort = 10022;

  // 0) 若上次中断残留 e2e-acc 或 10022，先清理
  await nav(page, "账户").click();
  const staleAccBtn = page.getByRole("button", { name: "账户 e2e-acc 操作" });
  if (await staleAccBtn.isVisible().catch(() => false)) {
    await staleAccBtn.click();
    await page.getByRole("menuitem", { name: "删除" }).click();
    const delAcc0 = page.getByRole("alertdialog");
    await delAcc0.getByRole("button", { name: "删除" }).click();
    await expect(delAcc0).toBeHidden();
  }

  // 1) 先新建端口 10022
  await nav(page, "端口线路").click();
  await deletePortRow(page, accPort);
  await page.getByRole("button", { name: "新增线路" }).first().click();
  const portForm = page.getByRole("dialog", { name: "新增端口线路" });
  await portForm.getByLabel("监听端口").fill(String(accPort));
  await portForm.getByRole("button", { name: "保存", exact: true }).click();
  await expect(portForm).toBeHidden();
  const created = page.getByRole("dialog", { name: "默认账号已生成" });
  if (await created.isVisible().catch(() => false)) {
    await created.getByRole("button", { name: "知道了" }).click();
    await expect(created).toBeHidden();
  }

  // 2) 进入账户管理，新建自定义账户 e2e-acc
  await nav(page, "账户").click();
  await expect(page.getByRole("heading", { level: 1, name: "账户" })).toBeVisible();
  await page.getByRole("button", { name: "新增账户" }).first().click();
  const accDialog = page.getByRole("dialog", { name: "新增账户" });
  await expect(accDialog).toBeVisible();
  await accDialog.locator("#acc-username").fill("e2e-acc");
  await accDialog.locator("#acc-password").fill("e2e-pass");
  await accDialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(accDialog).toBeHidden();
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-acc" })).toHaveCount(1);

  // 3) 打开复制接入向导并切换格式验证预览
  await page.getByRole("button", { name: "账户 e2e-acc 操作" }).click();
  await page.getByRole("menuitem", { name: "复制接入" }).click();
  const copyDialog = page.getByRole("dialog", { name: "复制接入信息" });
  await expect(copyDialog).toBeVisible();
  const preview = copyDialog.locator("#copy-preview");
  await expect(preview).toHaveValue(/http:\/\/e2e-acc:e2e-pass@/);
  await copyDialog.getByRole("button", { name: /SOCKS5/ }).first().click();
  await expect(preview).toHaveValue(/socks5h:\/\/e2e-acc:e2e-pass@/);
  await copyDialog.getByRole("button", { name: "关闭" }).click();
  await expect(copyDialog).toBeHidden();

  // 4) 删除自定义账户 e2e-acc，再清理端口 10022
  await page.getByRole("button", { name: "账户 e2e-acc 操作" }).click();
  await page.getByRole("menuitem", { name: "删除" }).click();
  const delAcc = page.getByRole("alertdialog");
  await expect(delAcc).toBeVisible();
  await delAcc.getByRole("button", { name: "删除" }).click();
  await expect(delAcc).toBeHidden();
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-acc" })).toHaveCount(0);

  await nav(page, "端口线路").click();
  await deletePortRow(page, accPort);
  await expectNoCrash(page, failures);
});

test("系统设置与连接日志：保存配置与日志筛选", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  await nav(page, "系统设置").click();
  await expect(page.getByRole("heading", { level: 1, name: "系统设置" })).toBeVisible();
  await page.getByRole("button", { name: "保存系统设置" }).click();
  await expect(page.locator("[data-sonner-toast]")).toContainText(/已保存/);
  await expectNoCrash(page, failures);

  await nav(page, "连接日志").click();
  await expect(page.getByRole("heading", { level: 1, name: "连接日志" })).toBeVisible();
  await page.locator("#log-level").click();
  await page.getByRole("option", { name: "warn 及以上" }).click();
  await expectNoCrash(page, failures);
});
