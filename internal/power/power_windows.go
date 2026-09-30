//go:build windows

package power

import (
	"syscall"
	"unsafe"
)

// systemPowerStatus is the SYSTEM_POWER_STATUS the call fills in.
type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

const (
	acLineOffline = 0

	// batteryCharging is the flag bit that says current is going in.
	batteryCharging = 8
	// noSystemBattery is the flag value a machine with no battery reports.
	noSystemBattery = 128
	// unknownPercentage is the percentage a machine that cannot measure its
	// charge reports.
	unknownPercentage = 255
)

// read asks Windows for the power status of the machine.
func read() (Status, error) {
	getSystemPowerStatus := syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

	var status systemPowerStatus
	ok, _, err := getSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		return Status{}, err
	}
	if status.BatteryFlag == noSystemBattery || status.BatteryLifePercent == unknownPercentage {
		return Status{}, nil
	}
	return Status{
		Percent:   int(status.BatteryLifePercent),
		OnBattery: status.ACLineStatus == acLineOffline,
		Charging:  status.BatteryFlag&batteryCharging != 0,
		Known:     true,
	}, nil
}
