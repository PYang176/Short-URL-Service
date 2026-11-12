param(
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

if (-not (Test-Path (Join-Path $root "deploy\.env"))) {
    Copy-Item `
        -LiteralPath (Join-Path $root "deploy\.env.example") `
        -Destination (Join-Path $root "deploy\.env")
    Write-Host "已创建 deploy\.env，请填写 MYSQL_DSN、JWT_SECRET 和 ANALYTICS_HASH_SECRET。"
    exit 1
}

if (-not $NoBuild) {
    Push-Location (Join-Path $root "web")
    try {
        npm run build
    } finally {
        Pop-Location
    }
}

docker compose `
    --env-file (Join-Path $root "deploy\.env") `
    -f (Join-Path $root "deploy\docker-compose.yml") `
    up -d --build
