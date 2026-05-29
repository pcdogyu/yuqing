@echo off
setlocal
cd /d "%~dp0"

start "auth-service" cmd /k go run .\cmd\auth-service
start "content-service" cmd /k go run .\cmd\content-service
start "crawler-service" cmd /k go run .\cmd\crawler-service
start "analysis-service" cmd /k go run .\cmd\analysis-service
start "nlp-service" cmd /k go run .\cmd\nlp-service
start "gateway-web" cmd /k go run .\cmd\gateway-web
start "scheduler-service" cmd /k go run .\cmd\scheduler-service

echo Services started. Open http://127.0.0.1:8080
