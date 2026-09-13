@echo off
REM ---------------------------------------------------------------------------
REM  Turn on automatic reporting.
REM
REM  DOUBLE-CLICK THIS FILE. There is nothing to type.
REM
REM  WHY IT EXISTS: scheduling needs the --install flag, and a flag cannot
REM  be passed by double-clicking the program - so without this file the one
REM  step that makes the tool do its job was the one step that required a
REM  command prompt. That is backwards for a tool whose whole point is that an
REM  owner never opens one.
REM ---------------------------------------------------------------------------

REM  The folder this file is in, not whatever folder Windows happened to start
REM  us in. /d so it works when the folder is on another drive.
cd /d "%~dp0"

if not exist "asa-pop.exe" (
  echo.
  echo   asa-pop.exe is not in this folder.
  echo.
  echo   This file and asa-pop.exe are downloaded separately, so they have
  echo   probably ended up in different places. Move this file into the SAME
  echo   folder as asa-pop.exe and double-click it again.
  echo.
  echo   This folder is:
  echo   %~dp0
  echo.
  pause
  exit /b 1
)

asa-pop.exe --install

REM  PAUSE, ALWAYS. Without it the window closes on the last line and an owner
REM  sees a flash - which is indistinguishable from nothing having happened,
REM  and the install prints the one warning that matters (it only runs while
REM  you are logged on).
echo.
pause
