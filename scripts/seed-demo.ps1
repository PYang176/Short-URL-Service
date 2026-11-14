param(
    [string]$BaseURL = "http://127.0.0.1:18080",
    [string]$Email = "demo@example.com",
    [string]$Password,
    [int]$HistoryDays = 7,
    [switch]$ResetHistory,
    [switch]$AllowRemote,
    [switch]$SkipClicks
)

$ErrorActionPreference = "Stop"

$baseUri = [Uri]$BaseURL
if (-not $baseUri.IsAbsoluteUri) {
    throw "BaseURL 必须是绝对地址。"
}
if (-not $AllowRemote -and $baseUri.Host -notin @("127.0.0.1", "localhost", "::1")) {
    throw "默认只允许向本机演示环境创建数据；远程执行必须显式传入 -AllowRemote。"
}
if ([string]::IsNullOrWhiteSpace($Password)) {
    $passwordBytes = New-Object byte[] 24
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($passwordBytes)
    $Password = [Convert]::ToBase64String($passwordBytes).TrimEnd("=").Replace("+", "-").Replace("/", "_")
}

$loginBody = @{ email = $Email; password = $Password } | ConvertTo-Json
$loginResponse = Invoke-WebRequest `
    -Method Post `
    -Uri "$BaseURL/api/v1/auth/login" `
    -ContentType "application/json" `
    -Body $loginBody `
    -SkipHttpErrorCheck

if ($loginResponse.StatusCode -eq 200) {
    $auth = $loginResponse.Content | ConvertFrom-Json
} else {
    $registerBody = @{
        email = $Email
        password = $Password
        display_name = "演示账号"
    } | ConvertTo-Json
    $registerResponse = Invoke-WebRequest `
        -Method Post `
        -Uri "$BaseURL/api/v1/auth/register" `
        -ContentType "application/json" `
        -Body $registerBody `
        -SkipHttpErrorCheck
    if ($registerResponse.StatusCode -ne 201) {
        throw "无法登录或创建演示账号: $($registerResponse.Content)"
    }
    $auth = $registerResponse.Content | ConvertFrom-Json
}

$headers = @{
    Authorization = "Bearer $($auth.token.access_token)"
}

$demoLinks = @(
    @{ alias = "launch"; title = "产品发布"; destination_url = "https://example.com/launch" },
    @{ alias = "docs"; title = "项目文档"; destination_url = "https://example.com/docs" },
    @{ alias = "campaign"; title = "营销活动"; destination_url = "https://example.com/campaign?utm_source=newsletter" },
    @{ alias = "github"; title = "GitHub 仓库"; destination_url = "https://github.com/PYang176/Short-URL-Service" }
)

$createdLinks = @()
foreach ($item in $demoLinks) {
    $requestHeaders = $headers.Clone()
    $requestHeaders["Idempotency-Key"] = "seed-$($item.alias)"
    $body = $item | ConvertTo-Json
    $response = Invoke-WebRequest `
        -Method Post `
        -Uri "$BaseURL/api/v1/links" `
        -Headers $requestHeaders `
        -ContentType "application/json" `
        -Body $body `
        -SkipHttpErrorCheck

    if ($response.StatusCode -eq 201) {
        $createdLinks += $response.Content | ConvertFrom-Json
        continue
    }

    # 已存在时从列表中找到同一个固定短码，保证脚本可重复执行。
    $list = Invoke-RestMethod `
        -Method Get `
        -Uri "$BaseURL/api/v1/links?limit=100" `
        -Headers $headers
    $existing = $list.items | Where-Object { $_.slug -eq $item.alias } | Select-Object -First 1
    if ($existing) {
        $createdLinks += $existing
        continue
    }
    throw "创建短链 $($item.alias) 失败: $($response.Content)"
}

if (-not $SkipClicks) {
    foreach ($link in $createdLinks) {
        curl.exe -s -o NUL -A "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0.0.0 Safari/537.36" $link.short_url
        curl.exe -s -o NUL -A "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0.0.0 Safari/537.36" $link.short_url
        curl.exe -s -o NUL -A "Googlebot/2.1" $link.short_url
    }
}

if ($HistoryDays -gt 0) {
    if ([string]::IsNullOrWhiteSpace($env:SHORTURL_MYSQL_DSN)) {
        Write-Warning "未设置 SHORTURL_MYSQL_DSN，跳过历史趋势数据。"
    } else {
        Push-Location (Split-Path -Parent $PSScriptRoot)
        try {
            $historyArgs = @(
                "run",
                ".\cmd\seed-demo-history",
                "-email",
                $Email,
                "-days",
                "$HistoryDays"
            )
            if ($ResetHistory) {
                $historyArgs += "-reset"
            }
            go @historyArgs
            if ($LASTEXITCODE -ne 0) {
                throw "生成演示历史数据失败，退出码: $LASTEXITCODE"
            }
        } finally {
            Pop-Location
        }
    }
}

$createdLinks |
    Select-Object title, slug, short_url, destination_url, status |
    Format-Table -AutoSize

Write-Host "演示账号: $Email"
Write-Host "演示密码: $Password"
Write-Host "请等待 Worker 消费点击事件后再打开分析页面。"
