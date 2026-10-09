//go:build darwin

package power

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The properties of a power source description this file reads, and the values
// two of them are compared against.
//
// IOPSKeys.h defines these as string literals rather than exported symbols, so
// there is nothing to look up at run time: they are written out here the way C
// code gets them from the header.
const (
	keyCurrentCapacity  = "Current Capacity"
	keyIsPresent        = "Is Present"
	keyPowerSourceState = "Power Source State"
	keyType             = "Type"

	valueBatteryPower = "Battery Power"
	valueInternalType = "InternalBattery"
)

// The framework entry points, resolved once. Loading a framework is an
// environment condition that can fail, so it is deliberately not an init(): the
// failure is reported to the caller, which plays the animation anyway.
var (
	loadOnce sync.Once
	loadErr  error

	copyPowerSourcesInfo func() objc.ID
	copyPowerSourcesList func(objc.ID) objc.ID
	getPowerSourceDesc   func(objc.ID, objc.ID) objc.ID
	cfRelease            func(objc.ID)

	selCount                objc.SEL
	selObjectAtIndex        objc.SEL
	selObjectForKey         objc.SEL
	selIntValue             objc.SEL
	selUTF8String           objc.SEL
	selStringWithUTF8String objc.SEL
)

// read reports the internal battery, if the machine has one.
func read() (Status, error) {
	if err := load(); err != nil {
		return Status{}, err
	}

	blob := copyPowerSourcesInfo()
	if blob == 0 {
		return Status{}, nil
	}
	defer cfRelease(blob)

	list := copyPowerSourcesList(blob)
	if list == 0 {
		return Status{}, nil
	}
	defer cfRelease(list)

	// A laptop lists one power source and a desktop lists none. A machine with
	// a UPS attached to it can list two, so the internal battery is picked out
	// by type rather than by position, and anything else is only used if no
	// battery was found at all.
	var other objc.ID
	for i := range objc.Send[int](list, selCount) {
		source := objc.Send[objc.ID](list, selObjectAtIndex, i)
		description := getPowerSourceDesc(blob, source)
		if description == 0 {
			continue
		}
		if text(description, keyType) == valueInternalType {
			return describe(description), nil
		}
		if other == 0 {
			other = description
		}
	}
	if other == 0 {
		return Status{}, nil
	}
	return describe(other), nil
}

// describe reads one power source description.
func describe(description objc.ID) Status {
	percent, ok := number(description, keyCurrentCapacity)
	if !ok {
		return Status{}
	}
	present, _ := number(description, keyIsPresent)
	return Status{
		Percent:   percent,
		OnBattery: text(description, keyPowerSourceState) == valueBatteryPower,
		Known:     present != 0,
	}
}

// number reads an integer property, reporting whether it was there at all.
func number(description objc.ID, key string) (int, bool) {
	value := objc.Send[objc.ID](description, selObjectForKey, nsstring(key))
	if value == 0 {
		return 0, false
	}
	return objc.Send[int](value, selIntValue), true
}

// text reads a string property.
func text(description objc.ID, key string) string {
	value := objc.Send[objc.ID](description, selObjectForKey, nsstring(key))
	if value == 0 {
		return ""
	}
	// UTF8String points into the string's own storage, which lives as long as
	// the description the caller releases it with.
	chars := objc.Send[unsafe.Pointer](value, selUTF8String)
	if chars == nil {
		return ""
	}
	var out []byte
	for i := 0; ; i++ {
		c := *(*byte)(unsafe.Add(chars, i))
		if c == 0 {
			break
		}
		out = append(out, c)
	}
	return string(out)
}

// nsstring makes an NSString around s, which is how the description dictionary
// is asked for anything. purego passes a Go string as a NUL terminated C string
// and keeps it alive for the call.
func nsstring(s string) objc.ID {
	return objc.Send[objc.ID](objc.ID(objc.GetClass("NSString")), selStringWithUTF8String, s)
}

// load opens the frameworks and resolves the entry points, once.
func load() error {
	loadOnce.Do(func() {
		loadErr = open()
	})
	return loadErr
}

func open() error {
	iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("power: dlopen IOKit: %w", err)
	}
	// The descriptions are CoreFoundation dictionaries, which are toll-free
	// bridged to their Objective-C counterparts: they are read with the
	// messages above rather than with the CF accessors. What they still need is
	// releasing, because every Copy call returns a reference that is ours.
	coreFoundation, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("power: dlopen CoreFoundation: %w", err)
	}

	// RegisterLibFunc dereferences the handle it is given, so a failed dlopen
	// has to stop here rather than leave the entry points pointing at a null
	// library.
	purego.RegisterLibFunc(&copyPowerSourcesInfo, iokit, "IOPSCopyPowerSourcesInfo")
	purego.RegisterLibFunc(&copyPowerSourcesList, iokit, "IOPSCopyPowerSourcesList")
	purego.RegisterLibFunc(&getPowerSourceDesc, iokit, "IOPSGetPowerSourceDescription")
	purego.RegisterLibFunc(&cfRelease, coreFoundation, "CFRelease")

	selCount = objc.RegisterName("count")
	selObjectAtIndex = objc.RegisterName("objectAtIndex:")
	selObjectForKey = objc.RegisterName("objectForKey:")
	selIntValue = objc.RegisterName("intValue")
	selUTF8String = objc.RegisterName("UTF8String")
	selStringWithUTF8String = objc.RegisterName("stringWithUTF8String:")
	return nil
}
