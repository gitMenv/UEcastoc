@pushd %~dp0
@if exist "packed" goto 1
mkdir "packed"
timeout -t 1 -nobreak
:1
@echo off
@if exist "manifest\manifest.json" goto 2
color 40
cls
echo manifest.json missing!
pause
:2
@echo on
@if "%~1"=="" goto skip
@pushd %~dp0
.\main.exe pack "%~dpn1" "manifest\manifest.json" "packed\%~n1" none
:skip