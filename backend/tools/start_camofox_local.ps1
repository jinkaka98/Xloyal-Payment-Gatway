$ErrorActionPreference = "Stop"
$env:CAMOFOX_HOST = "127.0.0.1"
$env:CAMOFOX_PORT = "9377"
$env:CAMOFOX_AUTH_MODE = "auto"
$env:CAMOFOX_PROFILES_DIR = "$env:LOCALAPPDATA\xloyal-payment-gateway\camofox-browser-profiles"

$log = "$env:LOCALAPPDATA\xloyal-payment-gateway\camofox-browser.log"
& "C:\Program Files\nodejs\node.exe" "$PSScriptRoot\camofox-browser\dist\src\server.js" *>> $log
