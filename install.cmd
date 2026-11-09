@echo off
rem Build and permanently register cusic in the current user's PATH.
rem Run from any working directory: call the installer next to this file.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\install-windows.ps1"
exit /b %ERRORLEVEL%
