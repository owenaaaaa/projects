@echo off
REM Build script for Windows - creates a hidden window executable
echo Building implant with hidden window...

go build -ldflags="-H windowsgui -s -w" -o servicehandler.exe servicehandler.go

if %ERRORLEVEL% EQU 0 (
    echo [*] built
) else (
    echo [!] Build failed
    exit /b 1
)
