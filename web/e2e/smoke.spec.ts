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
 * Data: the suite creates `10321` (port) and two `#e2e-*` upstreams through the
 * UI and removes them again. CI starts with a fresh DB file every run, and the
 * cleanup below also makes reruns against the same DB work.
 */

const PORT = 10321;
const UPSTREAM_1 = "http://u:p@1.1.1.1:8080#e2e-1";
const UPSTREAM_2 = "socks5://2.2.2.2:1080#e2e-2";
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? "";

const NAV: { label: string; title: string }[] = [
  { label: "端口线路", title: "端口线路" },
  { label: "账户", title: "账户" },
  { label: "上游代理", title: "上游代理" },
  { label: "连接日志", title: "连接日志" },
  { label: "系统设置", title: "系统设置" },
  { label: "仪表盘", title: "仪表盘" },
];

test.skip(!ADMIN_PASSWORD, "ADMIN_PASSWORD 未设置：需要真实后端密码才能跑 e2e");

/** React #31 / crashed tree guard: no error boundary text, no page errors. */
function watchRuntime(page: Page) {
  const failures: string[] = [];
  page.on("pageerror", (e) => failures.push(`pageerror: ${e.message}`));
  return failures;
}

async function expectNoCrash(page: Page, failures: string[]) {
  await expect(page.getByText("页面渲染出错")).toHaveCount(0);
  await expect(page.getByText("Minified React error")).toHaveCount(0);
  expect(failures, "浏览器运行时错误").toEqual([]);
}

async function login(page: Page) {
  await page.goto("/");
  const pw = page.getByLabel("管理密码");
  await expect(pw).toBeVisible();
  await pw.fill(ADMIN_PASSWORD);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible({ timeout: 20_000 });
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
  await row.getByRole("button", { name: /操作菜单$/ }).click();
  await row.page().getByRole("menuitem", { name: label, exact: true }).click();
}

async function deletePortRow(page: Page, port: number) {
  const rows = page.locator("tbody tr").filter({ hasText: String(port) });
  if ((await rows.count()) === 0) return;
  await rowAction(rows.first(), "删除");
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "删除", exact: true }).click();
  await expect(dialog).toBeHidden();
}

async function deleteUpstreamByName(page: Page, name: string) {
  const row = page.locator("tbody tr").filter({ has: page.getByText(name, { exact: true }) });
  if ((await row.count()) === 0) return false;
  await rowAction(row.first(), "删除");
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "删除", exact: true }).click();
  await expect(dialog).toBeHidden();
  return true;
}

test.describe.configure({ mode: "serial" });

test.afterEach(async ({ page }) => {
  // Best-effort cleanup so reruns against a reused DB stay green.
  try {
    if (ADMIN_PASSWORD) {
      await goTo(page, "upstreams");
      if (!(await page.getByLabel("搜索上游").isVisible().catch(() => false))) {
        await login(page);
        await goTo(page, "upstreams");
      }
      await deleteUpstreamByName(page, "e2e-1");
      await deleteUpstreamByName(page, "e2e-2");
      await deleteUpstreamByName(page, "e2e-ss");
      await deleteUpstreamByName(page, "e2e-vless");
      await goTo(page, "ports");
      await deletePortRow(page, PORT);
    }
  } catch {
    /* cleanup must never mask the real failure */
  }
});

test("登录页与错误密码提示", async ({ page }) => {
  const failures = watchRuntime(page);
  await page.goto("/");
  await expect(page.getByText("FastProxy 控制台")).toBeVisible();
  await expect(page.getByRole("heading", { name: /FastProxy/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();

  await page.getByLabel("管理密码").fill("definitely-wrong-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("密码错误");
  await expectNoCrash(page, failures);
});

test("正确密码登录后进入仪表盘", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  await expect(page.getByRole("heading", { level: 1, name: "仪表盘" })).toBeVisible();
  await expect(page.getByText("端口线路", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("上游健康", { exact: true })).toBeVisible();
  await expect(page.locator("aside nav")).toBeVisible();
  await expectNoCrash(page, failures);
});

test("六个导航页都能渲染（React #31 兜底）", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);

  for (const item of NAV) {
    await nav(page, item.label).click();
    await expect(page.getByRole("heading", { level: 1, name: item.title })).toBeVisible();
    await expect(page.locator("main")).not.toBeEmpty();
    await expectNoCrash(page, failures);
  }
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
  await expect(page.getByText("e2e-1", { exact: true })).toHaveCount(0);
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

  // VLESS：字段切换 + TLS 开关（关闭后 SNI 隐藏）。
  await page.getByRole("button", { name: "添加上游" }).first().click();
  const form2 = page.getByRole("dialog", { name: "添加上游代理" });
  await expect(form2).toBeVisible();
  await form2.getByLabel("名称").fill("e2e-vless");
  await form2.getByLabel("协议").click();
  await page.getByRole("option", { name: "VLESS" }).click();
  await form2.getByLabel("服务器地址").fill("2.2.2.2:443");
  await form2.getByLabel("UUID").fill("b831381d-6324-4d53-ad4f-8cda48b30811");
  await expect(form2.getByLabel("SNI")).toBeVisible();
  await form2.getByRole("switch", { name: "启用 TLS" }).click();
  await expect(form2.getByLabel("SNI")).toHaveCount(0);
  await form2.getByRole("button", { name: "保存", exact: true }).click();
  await expect(form2).toBeHidden();

  const vlessRow = page.locator("tbody tr").filter({ hasText: "e2e-vless" });
  await expect(vlessRow).toHaveCount(1);
  await expect(vlessRow.getByText("VLESS", { exact: true })).toBeVisible();

  // 编辑回填：按类型回填字段，而不是拼 ss:// 地址。
  await rowAction(ssRow.first(), "编辑");
  const edit = page.getByRole("dialog", { name: "编辑上游 e2e-ss" });
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

test("端口线路：新增/删除 10321 与每页下拉宽度", async ({ page }) => {
  const failures = watchRuntime(page);
  await login(page);
  await nav(page, "端口线路").click();
  await expect(page.getByRole("heading", { level: 1, name: "端口线路" })).toBeVisible();

  // 1) 新增线路
  await page.getByRole("button", { name: "新增线路" }).first().click();
  const form = page.getByRole("dialog", { name: "新增端口线路" });
  await expect(form).toBeVisible();
  await form.getByLabel("监听端口").fill(String(PORT));
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

  // 3) 删除 10321 清理
  await deletePortRow(page, PORT);
  await expect(page.locator("tbody tr").filter({ hasText: String(PORT) })).toHaveCount(0);
  await expectNoCrash(page, failures);
});
