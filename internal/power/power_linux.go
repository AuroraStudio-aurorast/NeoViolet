//go:build linux

package power

import (
	"os"
	"path/filepath"
	"strconv"
)

// sysfs is where the kernel publishes the power supplies it knows about. A
// machine with no battery has nothing of the sort in there.
const sysfs = "/sys/class/power_supply"

// read reports the first battery the kernel lists.
func read() (Status, error) {
	entries, err := os.ReadDir(sysfs)
	if err != nil {
		return Status{}, err
	}
	for _, entry := range entries {
		dir := filepath.Join(sysfs, entry.Name())
		if value(dir, "type") != "Battery" {
			continue
		}
		percent, err := strconv.Atoi(value(dir, "capacity"))
		if err != nil {
			continue
		}
		state := value(dir, "status")
		return Status{
			Percent:   percent,
			OnBattery: state == "Discharging",
			Known:     true,
		}, nil
	}
	return Status{}, nil
}

// value reads one attribute of a power supply, or "" if it is not there.
func value(dir, name string) string {
	// #nosec G304 -- a path under sysfs, built from a directory the kernel
	// publishes rather than from anything a user typed.
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return string(trimSpace(data))
}

// trimSpace trims the trailing newline sysfs attributes end with.
func trimSpace(data []byte) []byte {
	for len(data) > 0 {
		last := data[len(data)-1]
		if last != '\n' && last != '\r' && last != ' ' && last != '\t' {
			break
		}
		data = data[:len(data)-1]
	}
	return data
}
