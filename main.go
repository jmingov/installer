//
// Copyright 2017 The Maru OS Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/maruos/installer/android"
	"github.com/maruos/installer/remote"
	"github.com/pdsouza/toolbox.go/ui"
	"os"
	"path"
	"runtime"
	"time"
)

const (
	// Success exit codes.
	SuccessBase = 1<<5 + iota
	SuccessUserAbort
	SuccessBootloaderUnlocked

	Success = 0
)

const (
	// Error exit codes.
	ErrorBase = 1<<6 + iota
	ErrorPrereqs
	ErrorUserInput
	ErrorUsbPerms
	ErrorAdb
	ErrorFastboot
	ErrorRemote
	ErrorTWRP
)

var (
	reader      = bufio.NewReader(os.Stdin)
	progressBar = ui.ProgressBar{0, 10, ""}
)

func iEcho(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
}

func eEcho(msg string) {
	iEcho(msg)
}

func verifyAdbStatusOrAbort(adb *android.AdbClient) {
	status, err := adb.Status()
	if err != nil {
		eEcho("Failed to get adb status: " + err.Error())
		exit(ErrorAdb)
	}
	if status == android.NoDeviceFound || status == android.DeviceUnauthorized {
		eEcho(MsgAdbIssue)
		exit(ErrorAdb)
	} else if status == android.NoUsbPerms {
		eEcho(MsgFixPerms)
		exit(ErrorUsbPerms)
	}
}

func verifyFastbootStatusOrAbort(fastboot *android.FastbootClient) {
	status, err := fastboot.Status()
	if err != nil {
		eEcho("Failed to get fastboot status: " + err.Error())
		exit(ErrorFastboot)
	}
	if status == android.NoDeviceFound {
		eEcho(MsgFastbootNoDeviceFound)
		exit(ErrorFastboot)
	} else if status == android.NoUsbPerms {
		eEcho(MsgFixPerms)
		exit(ErrorUsbPerms)
	}
}

func progressCallback(percent float64) {
	progressBar.Progress = percent
	fmt.Print("\r" + progressBar.Render())
	if percent == 1.0 {
		fmt.Println()
	}
}

func exit(code int) {
	// When run by double-clicking the executable on windows, the command
	// prompt will immediately exit upon program completion, making it hard for
	// users to see the last few messages. Let's explicitly wait for
	// acknowledgement from the user.
	if runtime.GOOS == "windows" {
		fmt.Print("\nPress [Enter] to exit...")
		reader.ReadLine() // pause until the user presses enter
	}

	os.Exit(code)
}

func main() {
	var versionFlag = flag.Bool("version", false, "print the program version")
	flag.Parse()
	if *versionFlag == true {
		iEcho("Maru installer version %s %s/%s", Version, runtime.GOOS, runtime.GOARCH)
		exit(Success)
	}

	myPath, err := os.Executable()
	if err != nil {
		panic(err)
	}

	// include any bundled binaries in PATH
	err = os.Setenv("PATH", path.Dir(myPath)+":"+os.Getenv("PATH"))
	if err != nil {
		eEcho("Failed to set PATH to include installer tools: " + err.Error())
		exit(ErrorPrereqs)
	}

	// try to use the installer dir as the workdir to make sure any temporary
	// files or downloaded dependencies are isolated to the installer dir
	if err = os.Chdir(path.Dir(myPath)); err != nil {
		eEcho("Warning: failed to change working directory")
	}

	adb := android.NewAdbClient()
	if _, err := adb.Status(); err != nil {
		eEcho("Failed to run adb: " + err.Error())
		eEcho(MsgIncompleteZip)
		exit(ErrorPrereqs)
	}

	fastboot := android.NewFastbootClient()
	if _, err := fastboot.Status(); err != nil {
		eEcho("Failed to run fastboot: " + err.Error())
		eEcho(MsgIncompleteZip)
		exit(ErrorPrereqs)
	}

	iEcho(MsgWelcome)

	fmt.Print("Are you ready to install Maru? (yes/no): ")
	responseBytes, _, err := reader.ReadLine()
	if err != nil {
		iEcho("Failed to read input: ", err.Error())
		exit(ErrorUserInput)
	}

	if "yes" != string(responseBytes) {
		iEcho("")
		iEcho("Aborting installation.")
		exit(SuccessUserAbort)
	}

	iEcho("")
	iEcho("Checking USB permissions...")
	status, _ := fastboot.Status()
	if status == android.NoDeviceFound {
		// We are in ADB mode (normal boot or recovery).

		verifyAdbStatusOrAbort(&adb)

		iEcho("Rebooting your device into bootloader...")
		err = adb.Reboot("bootloader")
		if err != nil {
			eEcho("Failed to reboot into bootloader: " + err.Error())
			exit(ErrorAdb)
		}

		time.Sleep(7000 * time.Millisecond)

		if status, err = fastboot.Status(); err != nil || status == android.NoDeviceFound {
			eEcho("Failed to reboot device into bootloader!")
			exit(ErrorAdb)
		}
	}

	// We are in fastboot mode (the bootloader).

	verifyFastbootStatusOrAbort(&fastboot)

	iEcho("Identifying your device...")
	product, err := fastboot.GetProduct()
	if err != nil {
		eEcho("Failed to get device product info: " + err.Error())
		exit(ErrorFastboot)
	}

	unlocked, err := fastboot.Unlocked()
	if err != nil {
		iEcho("Warning: unable to determine bootloader lock state: " + err.Error())
	}
	if !unlocked {
		iEcho("Unlocking bootloader, you will need to confirm this on your device...")
		err = fastboot.Unlock()
		if err != nil {
			eEcho("Failed to unlock bootloader: " + err.Error())
			exit(ErrorFastboot)
		}
		fastboot.Reboot()
		iEcho(MsgUnlockSuccess)
		exit(SuccessBootloaderUnlocked)
	}

	iEcho("Downloading the latest release for your device (%q)...", product)
	server := remote.NewGitHubClient()
	req, err := server.RequestLatestRelease(product)
	if err != nil {
		eEcho("Failed to request the latest release: " + err.Error())
		exit(ErrorRemote)
	}

	zip := req.Filename
	if _, err = os.Stat(zip); os.IsNotExist(err) { // skip if we already downloaded it
		progressBar.Title = zip
		req.ProgressHandler = func(percent float64) {
			progressBar.Progress = percent
			fmt.Print("\r" + progressBar.Render())
			if percent == 1.0 {
				fmt.Println()
			}
		}
		zip, err = req.Download()
		if err != nil {
			eEcho("") // extra newline in case progress bar didn't finish
			eEcho("Failed to download the latest release: " + err.Error())
			exit(ErrorRemote)
		}
	}

	iEcho("Downloading TWRP for your device...")
	req, err = remote.RequestTWRP(product)
	if err != nil {
		eEcho("Failed to request TWRP: " + err.Error())
		exit(ErrorRemote)
	}

	twrp := req.Filename
	if _, err = os.Stat(twrp); os.IsNotExist(err) { // skip if we already downloaded it
		progressBar.Title = twrp
		req.ProgressHandler = func(percent float64) {
			progressBar.Progress = percent
			fmt.Print("\r" + progressBar.Render())
			if percent == 1.0 {
				fmt.Println()
			}
		}
		twrp, err = req.Download()
		if err != nil {
			eEcho("") // extra newline in case progress bar didn't finish
			eEcho("Failed to download TWRP: " + err.Error())
			exit(ErrorRemote)
		}
	}

	// iEcho("EARLY ABORT!")
	// exit(1)

	iEcho("Temporarily booting TWRP to flash Maru update zip...")
	err = fastboot.Boot(twrp)
	if err != nil {
		eEcho("Failed to boot TWRP: " + err.Error())
		exit(ErrorTWRP)
	}

	time.Sleep(10000 * time.Millisecond)

	iEcho("Transferring the Maru update zip to your device (may take a few mins)...")
	if err = adb.Push(zip, "/sdcard"); err != nil {
		eEcho("Failed to push Maru update zip to device: " + err.Error())
		exit(ErrorAdb)
	}

	iEcho("Installing Maru, please keep your device connected...")
	err = adb.Shell("twrp install /sdcard/" + zip)
	if err != nil {
		eEcho("Failed to flash Maru update zip: " + err.Error())
		exit(ErrorTWRP)
	}

	// Pause a bit after install or TWRP gets confused
	time.Sleep(2000 * time.Millisecond)

	iEcho("Wiping your device without wiping /data/media...")
	err = adb.Shell("twrp wipe cache")
	if err != nil {
		eEcho("Failed to wipe cache: " + err.Error())
		exit(ErrorTWRP)
	}
	time.Sleep(1000 * time.Millisecond)
	err = adb.Shell("twrp wipe dalvik")
	if err != nil {
		eEcho("Failed to wipe dalvik: " + err.Error())
		exit(ErrorTWRP)
	}
	time.Sleep(1000 * time.Millisecond)
	err = adb.Shell("twrp wipe data")
	if err != nil {
		eEcho("Failed to wipe data: " + err.Error())
		exit(ErrorTWRP)
	}
	time.Sleep(1000 * time.Millisecond)

	iEcho(MsgSuccess)
	err = adb.Reboot("")
	if err != nil {
		eEcho("Failed to reboot: " + err.Error())
		iEcho("\nPlease reboot your device manually by going to Reboot > System > Do Not Install")
		exit(ErrorAdb)
	}

	exit(Success)
}