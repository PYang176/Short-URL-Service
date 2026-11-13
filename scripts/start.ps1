param(
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

if (-not (Test-Path (Join-Path $root "deploy\.env"))) {
    Copy-Item `
        -LiteralPath (Join-Path $root "deploy\.env.example") `
        -Destination (Join-Path $root "deploy\.env")
    Write-Host "已创建 deploy\.env，请填写 MYSQL_DSN、JWT_SECRET、ANALYTICS_HASH_SECRET 和 GRAFANA_ADMIN_PASSWORD。"
    exit 1
}

if (-not $NoBuild) {
    Push-Location (Join-Path $root "web")
    try {
        npm run build
    } finally {
        Pop-Location
    }

    docker build `
        --pull=false `
        --tag shorturl-app:local `
        --file (Join-Path $root "Dockerfile") `
        $root
} else {
    docker image inspect shorturl-app:local *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "镜像 shorturl-app:local 不存在，请去掉 -NoBuild 先构建镜像。"
    }
}

$composeArgs = @(
    "compose",
    "--env-file", (Join-Path $root "deploy\.env"),
    "-f", (Join-Path $root "deploy\docker-compose.yml"),
    "up", "-d"
)
if (-not $NoBuild) {
    $composeArgs += "--build"
}
docker @composeArgs
