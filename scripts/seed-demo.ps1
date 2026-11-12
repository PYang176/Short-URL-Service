param(
    [string]$BaseURL = "http://127.0.0.1:18080",
    [string]$Email = "demo@example.com",
    [string]$Password = "demo12345",
    [switch]$SkipClicks
)

$ErrorActionPreference = "Stop"

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

$createdLinks |
    Select-Object title, slug, short_url, destination_url, status |
    Format-Table -AutoSize

Write-Host "演示账号: $Email"
Write-Host "演示密码: $Password"
Write-Host "请等待 Worker 消费点击事件后再打开分析页面。"
