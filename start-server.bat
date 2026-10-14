@echo off
rem phanthycode2api launcher: double-click to run.
rem Logs show up in this window and are appended to logs\server.out.log at the same time.
rem If the service is already running, this only opens the admin page instead of starting a second copy.
chcp 65001 >nul
cd /d "%~dp0"
if not exist "logs" mkdir "logs"

rem If the port is already serving, just open the admin page and leave the running copy alone.
powershell -NoProfile -ExecutionPolicy Bypass -Command "$port='7864'; try { $cfg=ConvertFrom-Json (Get-Content -Raw 'config.json'); if ($cfg.listen) { $port=($cfg.listen -replace '^.*:','') } } catch {}; if ($port -notmatch '^\d+$') { $port='7864' }; $up=$false; try { $c=New-Object Net.Sockets.TcpClient; $c.Connect('127.0.0.1',[int]$port); $up=$true; $c.Close() } catch {}; if ($up) { Start-Process ('http://127.0.0.1:'+$port+'/admin'); exit 0 }; exit 1"
if not errorlevel 1 (
  echo phanthycode2api is already running. Opened the admin page in your browser.
  exit /b 0
)

powershell -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Continue'; [Console]::OutputEncoding=[System.Text.Encoding]::UTF8; & '.\phanthycode2api.exe' -config 'config.json' 2>&1 | ForEach-Object { $line = [string]$_ + ''; $line | Out-File -FilePath 'logs\server.out.log' -Append -Encoding utf8; Write-Host $line }"
echo.
echo phanthycode2api exited. Full log: logs\server.out.log
pause
