@ECHO OFF
::
:: Copyright (c) 2015 Maru
::

SETLOCAL ENABLEEXTENSIONS
SET /A ERROR_INSTALLER=1
SET /A ERROR_RECOVERY=2
SET /A ERROR_UNLOCK=4
SET me=%~n0
SET parent_dir=%~dp0

ECHO.
ECHO Welcome to the Maru installer!
ECHO.
ECHO In order to install Maru you will need to:
ECHO.
ECHO 1. Connect your Nexus 5 to your computer over USB
ECHO. 
ECHO 2. Enable USB Debugging on your device:
ECHO.
ECHO    1.  Go to the Settings app and scroll down to
ECHO        the System section
ECHO.
ECHO        NOTE: If you already have "Developer options" under
ECHO        System then go directly to #5
ECHO.
ECHO    2.  Tap on "About phone"
ECHO    3.  Tap "Build number" 7 times until you get a message that says
ECHO        you are now a developer
ECHO    4.  Go back to the main Settings app
ECHO    5.  Tap on "Developer options"
ECHO    6.  Ensure that "USB debugging" is enabled
ECHO.
ECHO IMPORTANT: Installing Maru requires a factory reset of your device
ECHO so make sure you first back-up any important data!
ECHO.

SET /P confirm="Are you ready to install Maru? (yes/no): "
ECHO.
IF NOT "%confirm%"=="yes" (
    ECHO Aborting installation.
    CALL :mexit 0
)

ECHO Excellent, let's get Maru up and running on your device.
ECHO.

PUSHD %parent_dir%

ECHO Checking for a complete installation zip...
CALL :check_zip
IF /I "%ERRORLEVEL%" NEQ "0" (
    ECHO.
    ECHO Hmm, looks like your installer is missing a few things.
    ECHO.
    ECHO Are you running this install script outside the directory
    ECHO you unzipped Maru in?
    ECHO.
    ECHO If that isn't it, please try downloading the installer zip again.
    ECHO.
    CALL :mexit %ERROR_INSTALLER%
)

ECHO Checking whether your device is in recovery mode...
CALL :check_recovery
IF /I "%ERRORLEVEL%" EQU "0" (
    GOTO :bootloader
)

:recovery_adb
ECHO Rebooting your device into recovery mode...
adb reboot bootloader > NUL 2>&1
IF /I "%ERRORLEVEL%" NEQ "0" (
    ECHO.
    ECHO Hmm, your device can't be found.
    ECHO.
    ECHO Please ensure that:
    ECHO.
    ECHO 1. Your device is connected to your computer over USB
    ECHO 2. You have USB Debugging enabled (see above for instructions)
    ECHO 3. You unlock your device and tap "OK" if you see a dialog asking you
    ECHO    to allow USB Debugging for your computer's RSA key fingerprint
    ECHO.
    ECHO Go ahead and re-run the installer when you're ready.
    CALL :mexit %ERROR_RECOVERY%
)
PING.EXE -n 7 127.0.0.1 > NUL

ECHO Checking whether your device is in recovery mode...
CALL :check_recovery
IF /I "%ERRORLEVEL%" NEQ "0" (
    ECHO Your device isn't in recovery mode, let's try this the manual way.
    CALL :manual_recovery
)

:bootloader
ECHO Checking bootloader lock state...
CALL :check_unlocked
IF /I "%ERRORLEVEL%" EQU "0" (
    GOTO :flash
)

ECHO Unlocking bootloader, you will need to confirm this on your device...
fastboot oem unlock > NUL 2>&1
CALL :check_unlocked
IF /I "%ERRORLEVEL%" NEQ "0" (
    ECHO Failed to unlock bootloader!
    CALL :fatal
    CALL :mexit %ERROR_UNLOCK%
)

:: ECHO BAIL!
:: PAUSE
:: EXIT /B 0

:flash
ECHO.
ECHO Installing Maru, please keep your device connected...
fastboot format cache
fastboot flash boot boot.img
fastboot flash system system.img
fastboot flash userdata userdata.img

ECHO.
ECHO Rebooting into Maru...
fastboot reboot > NUL 2>&1

ECHO.
ECHO Installation success!
CALL :mexit 0


:: functions

:check_zip
IF NOT EXIST "boot.img" EXIT /B 1
IF NOT EXIST "system.img" EXIT /B 1
IF NOT EXIST "userdata.img" EXIT /B 1
EXIT /B 0

:check_recovery
FOR /F "tokens=* USEBACKQ" %%F IN (`fastboot devices`) DO (
    SET recovery_check=fastboot devices
)
IF "%recovery_check%"=="" (EXIT /B 1)
EXIT /B 0

:manual_recovery
ECHO.
ECHO Please reboot your device into recovery mode:
ECHO.
ECHO 1. Power off your device
ECHO 2. Hold down the Volume Up, Volume Down, and Power buttons on your
ECHO    device for a couple of seconds
ECHO.
SET /P ready="Hit ENTER when your device is in recovery mode: "
ECHO.
ECHO Checking whether your device is in recovery mode...
CALL :check_recovery
IF /I "%ERRORLEVEL%" NEQ "0" (
    ECHO Your device isn't in recovery mode, please try again.
    CALL :manual_recovery
)
EXIT /B 0

:check_unlocked
fastboot oem device-info 2>&1 | FINDSTR "unlocked" | FINDSTR "true"
IF /I "%ERRORLEVEL%" NEQ "0" (EXIT /B 1)
EXIT /B 0

:fatal
ECHO.
ECHO Yikes, something went wrong with the installation. We're sorry.
ECHO.
ECHO Please contact us at hello@maruos.com with the issue you are facing
ECHO and we'll personally walk you through the install process.
ECHO.
EXIT /B 0

:mexit
ECHO.
ECHO Press any key to exit...
PAUSE > NUL
POPD
EXIT %1
