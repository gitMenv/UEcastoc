@pushd %~dp0
@if exist "output" goto 1
mkdir "output"
:1
@if exist "manifest" goto 2
mkdir "manifest"
:2
timeout -t 1 -nobreak
@if "%~1"=="" goto skip
.\main.exe unpackAll "%~dpn1.utoc" "%~dpn1.ucas" output\
.\main.exe manifest "%~dpn1.utoc" "%~dpn1.ucas" "manifest\%~n1.json"
:skip