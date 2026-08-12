@echo off
set CAMOFOX_HOST=127.0.0.1
set CAMOFOX_PORT=9377
set CAMOFOX_AUTH_MODE=auto
set CAMOFOX_PROFILES_DIR=%LOCALAPPDATA%\xloyal-payment-gateway\camofox-browser-profiles
"C:\Program Files\nodejs\node.exe" "C:\Users\POWER-OF-MAGIC\Documents\GitHub\Xloyal-Payment-Gatway\backend\tools\camofox-browser\dist\src\server.js" >> "%LOCALAPPDATA%\xloyal-payment-gateway\camofox-browser.log" 2>&1
