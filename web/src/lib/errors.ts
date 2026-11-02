import type { Lang } from "@/i18n";

export type { Lang };

/**
 * 把后端返回的原始错误 / 网络错误映射成“人话 + 建议”。
 *
 * 用于：上游检测结果、批量导入的 problems、订阅拉取错误等。原始错误永远保留在
 * `raw` 里（可折叠展示），前端只负责在它前面补一句可读的原因和建议。
 */

export type ErrorHint = {
  /** 本地化后的简短原因。 */
  message: string;
  /** 本地化后的排查建议（可空）。 */
  hint: string;
  /** 命中的规则名，便于测试与调试。 */
  code: string;
  /** 后端原始错误文本。 */
  raw: string;
};

type Rule = {
  code: string;
  match: RegExp;
  zh: [string, string];
  en: [string, string];
};

// 顺序有意义：越具体的规则越靠前。
const RULES: Rule[] = [
  {
    code: "reality-keys",
    match: /reality.*(pbk|sid|short\s*id|public\s*key)|(pbk|sid).*reality/i,
    zh: [
      "REALITY 节点缺少密钥或密钥不匹配",
      "检查节点是否填写了 pbk / sid（短 ID 一般是 16 位十六进制），并与服务端 REALITY 配置逐位核对。",
    ],
    en: [
      "REALITY node is missing keys or the keys do not match",
      "Check that pbk / sid are filled in (short IDs are usually 16 hex chars) and match the server's REALITY config exactly.",
    ],
  },
  {
    code: "auth",
    match: /\b(401|407)\b|unauthorized|proxy\s*authentication|auth(entication)?\s*failed|invalid\s*(user|password|credential)/i,
    zh: [
      "上游账号或密码错误（HTTP 401/407）",
      "核对上游的用户名与密码；若使用了 ${VAR} 环境变量，确认容器里已注入。",
    ],
    en: [
      "Upstream username or password rejected (HTTP 401/407)",
      "Double-check the upstream credentials; if you used ${VAR} references, make sure they are set in the container.",
    ],
  },
  {
    code: "tls",
    match: /tls\s*handshake|x509|certificate|unknown\s*authority|hostname\s*mismatch|bad\s*certificate|ssl/i,
    zh: [
      "TLS 握手失败：证书 / SNI / REALITY 配置不匹配",
      "核对 SNI 是否是服务端证书覆盖的域名；REALITY 节点检查 pbk、sid 与 dest 是否与服务端一致。",
    ],
    en: [
      "TLS handshake failed: certificate / SNI / REALITY mismatch",
      "Check that the SNI is covered by the server certificate; for REALITY verify pbk, sid and dest match the server.",
    ],
  },
  {
    code: "timeout",
    match: /i\/o\s*timeout|timeout|timed\s*out|deadline\s*exceeded/i,
    zh: [
      "服务器到代理端口不通（超时）",
      "检查上游端口是否开放、防火墙/安全组是否放行本机 IP，部分机房对被墙或受限地域的连接会直接超时。",
    ],
    en: [
      "Server cannot reach the proxy port (timeout)",
      "Check the port is open and firewalls / security groups allow this server's IP; some networks drop connections from geo-restricted regions.",
    ],
  },
  {
    code: "refused",
    match: /connection\s*refused|actively\s*refused/i,
    zh: [
      "连接被拒绝：服务未监听或端口错误",
      "确认上游服务正在运行，且地址里的端口与协议（http / socks5）填写正确。",
    ],
    en: [
      "Connection refused: nothing is listening or the port is wrong",
      "Make sure the upstream service is running and that the port and protocol (http / socks5) are correct.",
    ],
  },
  {
    code: "eof",
    match: /\bEOF\b|connection\s*(reset|closed)|broken\s*pipe|unexpected\s*EOF/i,
    zh: [
      "连接被提前关闭（EOF / reset）",
      "常见于协议识别错误、TLS 被中间设备重置，或节点已失效；可先用客户端直连同一节点验证。",
    ],
    en: [
      "Connection closed unexpectedly (EOF / reset)",
      "Often a protocol mismatch, TLS reset by middleboxes, or a dead node; verify the node directly with a client first.",
    ],
  },
  {
    code: "dns",
    match: /no\s*route\s*to\s*host|network\s*is\s*unreachable|dial\s*tcp.*no\s*such\s*host|lookup\s+.*(failed|no such host)|name\s*resolution/i,
    zh: [
      "无法路由到目标（DNS / 网络不可达）",
      "检查地址是否拼写正确、域名能否解析；确认容器网络可以出站。",
    ],
    en: [
      "No route to host (DNS / network unreachable)",
      "Check the host spelling and DNS resolution, and that the container network allows outbound traffic.",
    ],
  },
  {
    code: "probe-target",
    match: /probe\s*target|ipify|get\s*exit\s*ip|exit\s*ip.*fail/i,
    zh: [
      "代理已连通，但获取出口 IP 失败",
      "可能是上游限制了探测目标域名；换一个探测目标域名再试。",
    ],
    en: [
      "Proxy connected but fetching the exit IP failed",
      "The upstream may block the probe target domain; try a different probe target.",
    ],
  },
  {
    code: "http-status",
    match: /\b(4\d\d|5\d\d)\b/,
    zh: [
      "上游返回了 HTTP 错误状态",
      "检查该 HTTP 代理是否需要认证、是否有访问限制（429 表示被限流）。",
    ],
    en: [
      "Upstream returned an HTTP error status",
      "Check whether the HTTP proxy requires auth or is rate limited (429).",
    ],
  },
  {
    code: "network",
    match: /failed\s*to\s*fetch|network\s*error|load\s*failed|networkerror/i,
    zh: ["无法连接控制台（网络错误）", "确认控制台服务仍在运行、网络正常，然后重试。"],
    en: ["Cannot reach the control panel (network error)", "Make sure the panel is still running and the network works, then retry."],
  },
  {
    code: "not-implemented",
    match: /^请求失败 \(HTTP (404|405|501)\)$|not\s*implemented|method\s*not\s*allowed/i,
    zh: ["当前后端不支持该接口", "升级后端到包含该接口的版本后再试。"],
    en: ["This backend does not provide that endpoint", "Upgrade the backend to a version that includes it and try again."],
  },
];

/** Matches an error string against the rule table. Returns the code of the first hit, or "". */
export function matchErrorCode(raw: string): string {
  const s = (raw || "").trim();
  if (!s) return "";
  for (const r of RULES) if (r.match.test(s)) return r.code;
  return "";
}

/** Localized reason + advice for an arbitrary error string. Never throws; unknown errors pass through. */
export function explainError(raw: unknown, lang: Lang): ErrorHint {
  const text = typeof raw === "string" ? raw : raw instanceof Error ? raw.message : String(raw ?? "");
  const s = text.trim();
  const code = matchErrorCode(s);
  if (!code) {
    return {
      code: "",
      raw: s,
      message: lang === "zh" ? s || "未知错误" : s || "Unknown error",
      hint: lang === "zh" ? "如果持续失败，请检查上游配置或查看服务端日志。" : "If this keeps happening, check the upstream config or the server logs.",
    };
  }
  const rule = RULES.find((r) => r.code === code)!;
  const [message, hint] = lang === "zh" ? rule.zh : rule.en;
  return { code, raw: s, message, hint };
}

/** One-line form for table cells / toasts: “原因（原始错误）”。 */
export function explainErrorLine(raw: unknown, lang: Lang): string {
  const e = explainError(raw, lang);
  if (!e.code) return e.message;
  return e.raw && e.raw !== e.message ? `${e.message}：${e.raw}` : e.message;
}

/** Import `problems` entries look like `line: detail` or just a share link + reason. */
export function explainProblem(problem: string, lang: Lang): ErrorHint {
  const e = explainError(problem, lang);
  if (e.code) return e;
  // 未命中规则时，导入问题通常包含节点长文本，截断显示。
  const short = problem.length > 120 ? `${problem.slice(0, 117)}…` : problem;
  return {
    ...e,
    message: short,
    hint: lang === "zh" ? "该行被跳过；检查是否为完整分享链接或 IP:PORT。" : "This line was skipped; check that it is a complete share link or IP:PORT.",
  };
}

/** Prefixed message for a failed request, e.g. “保存失败：端口超出范围”。 */
export function withPrefix(prefix: string, raw: unknown): string {
  const text = typeof raw === "string" ? raw : raw instanceof Error ? raw.message : String(raw ?? "");
  return text ? `${prefix}：${text}` : prefix;
}
