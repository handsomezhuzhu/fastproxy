# FastProxy 容器端到端联调脚本（Windows / PowerShell）
# 用法: .\scripts\e2e.ps1
$ErrorActionPreference = 'Stop'
$PASS = 'admin123'
$PROXY_PORT = 10201
$ADMIN_PORT = 17788

Write-Host '== 1. 交叉编译 linux/amd64 ==' -ForegroundColor Cyan
$env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
go build -trimpath -ldflags "-s -w" -o fastproxy-e2e-linux .
if ($LASTEXITCODE -ne 0) { throw 'build failed' }
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue

Write-Host '== 2. 启动本地 devserver (fake upstream + target) ==' -ForegroundColor Cyan
$devProc = Start-Process -FilePath 'go' -ArgumentList 'run', './scripts/devserver' `
  -RedirectStandardOutput 'devserver.log' -RedirectStandardError 'devserver.err' `
  -WindowStyle Hidden -PassThru
Start-Sleep -Seconds 3
Get-Content devserver.log
Get-Content devserver.err -ErrorAction SilentlyContinue

Write-Host '== 3. 构建并启动容器 ==' -ForegroundColor Cyan
docker rm -f fastproxy-e2e 2>$null | Out-Null
docker build -f Dockerfile.e2e -t fastproxy:e2e .
docker run -d --name fastproxy-e2e `
  -p "${ADMIN_PORT}:7788" -p "${PROXY_PORT}:${PROXY_PORT}" `
  -e ADMIN_PASSWORD=$PASS fastproxy:e2e | Out-Null

# 等面板就绪
$ready = $false
for ($i = 0; $i -lt 40; $i++) {
  try { Invoke-RestMethod "http://127.0.0.1:${ADMIN_PORT}/api/session" | Out-Null; $ready = $true; break }
  catch { Start-Sleep -Milliseconds 500 }
}
if (-not $ready) { docker logs fastproxy-e2e; throw 'panel not ready' }
Write-Host '   面板就绪' -ForegroundColor Green

Write-Host '== 4. 通过面板 API 配置（登录/上游/端口/账户） ==' -ForegroundColor Cyan
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:${ADMIN_PORT}/api/login" `
  -ContentType 'application/json' -Body ('{"password":"' + $PASS + '"}') -SessionVariable web | Out-Null

$up = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:${ADMIN_PORT}/api/upstreams" -WebSession $web `
  -ContentType 'application/json' -Body (@{ name = 'devserver-upstream'; upstream = 'http://host.docker.internal:18180' } | ConvertTo-Json)
$port = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:${ADMIN_PORT}/api/ports" -WebSession $web `
  -ContentType 'application/json' -Body (@{ port = $PROXY_PORT; default_upstream = $up.upstream.id } | ConvertTo-Json)
$acc = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:${ADMIN_PORT}/api/accounts" -WebSession $web `
  -ContentType 'application/json' -Body (@{ username = 'alice'; password = 'wonder'; port_id = $port.port.id } | ConvertTo-Json)
Write-Host ("   upstream={0} port={1} account={2}" -f $up.upstream.id, $port.port.port, $acc.account.username) -ForegroundColor Green

# 容器 -> 宿主机：先试 host.docker.internal，失败则热切换到 bridge 网关 IP
$upHost = 'host.docker.internal'
$probe = (curl.exe -s --max-time 8 -x "http://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")
if ($probe -notmatch 'hello-from-devserver-target') {
  $gw = (docker inspect fastproxy-e2e --format '{{(index .NetworkSettings.Networks "bridge").Gateway}}').Trim()
  Write-Host "   host.docker.internal 不通, 热切换上游到网关 $gw" -ForegroundColor Yellow
  $up = Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:${ADMIN_PORT}/api/upstreams/$($up.upstream.id)" -WebSession $web `
    -ContentType 'application/json' -Body (@{ name = 'devserver-upstream'; upstream = "http://${gw}:18180" } | ConvertTo-Json)
  $upHost = $gw
}
$probe = (curl.exe -s --max-time 8 -x "http://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")
if ($probe -notmatch 'hello-from-devserver-target') { docker logs fastproxy-e2e; throw "container cannot reach host devserver (tried host.docker.internal and $upHost)" }
Write-Host "   经上游 $upHost 连通 OK" -ForegroundColor Green

Write-Host '== 5. 代理连通性测试 ==' -ForegroundColor Cyan
$fail = 0

$http = (curl.exe -s --max-time 15 -x "http://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")
if ($http -match 'hello-from-devserver-target') { Write-Host '   [PASS] HTTP 明文代理 (经上游)' -ForegroundColor Green } else { Write-Host "   [FAIL] HTTP: $http" -ForegroundColor Red; $fail++ }

$socks = (curl.exe -s --max-time 15 --proxy "socks5h://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")
if ($socks -match 'hello-from-devserver-target') { Write-Host '   [PASS] SOCKS5 代理 (经上游)' -ForegroundColor Green } else { Write-Host "   [FAIL] SOCKS5: $socks" -ForegroundColor Red; $fail++ }

$tun = (curl.exe -s --max-time 15 -p -x "http://alice:wonder@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")
if ($tun -match 'hello-from-devserver-target') { Write-Host '   [PASS] CONNECT 隧道 (curl --proxytunnel)' -ForegroundColor Green } else { Write-Host "   [FAIL] CONNECT: $tun" -ForegroundColor Red; $fail++ }

$code407 = (curl.exe -s -o NUL -w '%{http_code}' --max-time 15 -x "http://alice:badpass@127.0.0.1:${PROXY_PORT}" "http://127.0.0.1:18181/")
if ($code407 -eq '407') { Write-Host '   [PASS] 错误密码返回 407' -ForegroundColor Green } else { Write-Host "   [FAIL] 期望 407 实际 $code407" -ForegroundColor Red; $fail++ }

$code401 = (curl.exe -s -o NUL -w '%{http_code}' --max-time 15 "http://127.0.0.1:${ADMIN_PORT}/api/data")
if ($code401 -eq '401') { Write-Host '   [PASS] 未登录访问 API 返回 401' -ForegroundColor Green } else { Write-Host "   [FAIL] 期望 401 实际 $code401" -ForegroundColor Red; $fail++ }

$bad = (curl.exe -s --max-time 15 -o NUL -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"password":"wrong"}' "http://127.0.0.1:${ADMIN_PORT}/api/login")
if ($bad -eq '401') { Write-Host '   [PASS] 面板错误密码登录返回 401' -ForegroundColor Green } else { Write-Host "   [FAIL] 期望 401 实际 $bad" -ForegroundColor Red; $fail++ }

$data = Invoke-RestMethod "http://127.0.0.1:${ADMIN_PORT}/api/data" -WebSession $web
$st = $data.status.ports | Where-Object { $_.id -eq $port.port.id }
if ($st.conns_total -ge 3 -and $st.bytes_in -gt 0) { Write-Host ("   [PASS] 统计累计: conns_total={0} bytes_in={1} bytes_out={2}" -f $st.conns_total, $st.bytes_in, $st.bytes_out) -ForegroundColor Green } else { Write-Host "   [FAIL] 统计异常: $($st | ConvertTo-Json -Compress)" -ForegroundColor Red; $fail++ }

Write-Host ''
if ($fail -eq 0) { Write-Host 'E2E 全部通过 ✔（容器保持运行: fastproxy-e2e，面板 http://127.0.0.1:17788，密码 admin123）' -ForegroundColor Green }
else { Write-Host ("E2E 失败 {0} 项 ✘" -f $fail) -ForegroundColor Red; docker logs fastproxy-e2e; exit 1 }
