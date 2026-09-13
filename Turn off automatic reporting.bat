@echo off
REM ---------------------------------------------------------------------------
REM  Turn off automatic reporting.
REM
REM  DOUBLE-CLICK THIS FILE. Your listing stops showing a live count within 25
REM  minutes and says so; the average already recorded stays, because that is
REM  history rather than a claim about now.
REM
REM  It ships WITH the turn-on file deliberately. An owner who can only start
REM  this by double-clicking, and would have to open a command prompt to stop
REM  it, has been handed a trap rather than a tool.
REM ---------------------------------------------------------------------------

cd /d "%~dp0"

if not exist "asa-pop.exe" (
  echo.
  echo   asa-pop.exe is not in this folder.
  echo   Move this file into the SAME folder as asa-pop.exe and try again.
  echo.
  pause
  exit /b 1
)

asa-pop.exe --uninstall

echo.
pause
