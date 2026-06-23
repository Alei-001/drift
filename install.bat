@echo off
setlocal

REM Drift installer for Windows
REM Builds and installs drift to %USERPROFILE%\bin (added to PATH if needed)

set VERSION=0.1.0
set INSTALL_DIR=%USERPROFILE%\bin

echo Building drift %VERSION%...

go build -ldflags "-X github.com/drift/drift/internal/cli.version=%VERSION%" -o drift.exe ./cmd/drift/
if errorlevel 1 (
    echo Build failed.
    exit /b 1
)

if not exist "%INSTALL_DIR%" (
    mkdir "%INSTALL_DIR%"
)

move /Y drift.exe "%INSTALL_DIR%\drift.exe" >nul
if errorlevel 1 (
    echo Install failed: could not move drift.exe to %INSTALL_DIR%
    exit /b 1
)

REM Add to PATH for current session if not already there
echo %PATH% | findstr /C:"%INSTALL_DIR%" >nul
if errorlevel 1 (
    setx PATH "%PATH%;%INSTALL_DIR%" >nul
    echo Added %INSTALL_DIR% to PATH (restart terminal to take effect)
)

echo.
echo drift %VERSION% installed to %INSTALL_DIR%\drift.exe
echo Run "drift version" to verify.
