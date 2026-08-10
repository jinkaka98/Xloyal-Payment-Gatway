$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

if (-not $env:CREDENTIAL_ENCRYPTION_KEY) {
    $env:CREDENTIAL_ENCRYPTION_KEY = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
}
docker compose --project-directory $root -f (Join-Path $root "compose.yaml") config --quiet
if ($LASTEXITCODE -ne 0) { throw "docker compose config failed with exit code $LASTEXITCODE" }

$spec = Join-Path $root "openapi/openapi.yaml"
node (Join-Path $PSScriptRoot "validate-openapi.cjs") $spec
if ($LASTEXITCODE -ne 0) { throw "OpenAPI validation failed with exit code $LASTEXITCODE" }

Write-Host "Compose config and OpenAPI syntax OK"
