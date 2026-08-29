//go:build darwin

package mediactl

import (
	"github.com/ebitengine/purego/objc"
)

// ObjC convenience helpers

func nsString(s string) objc.ID {
	id := objc.ID(classNSString).Send(selAlloc).Send(selInitWithUTF8String, s)
	id.Send(selAutorelease)
	return id
}

func nsInt(v int32) objc.ID {
	return objc.ID(classNSNumber).Send(selNumberWithInt, v)
}

func nsDouble(v float64) objc.ID {
	return objc.ID(classNSNumber).Send(selNumberWithDouble, v)
}

func nsMutableDict() objc.ID {
	id := objc.ID(classNSMutableDictionary).Send(selAlloc).Send(selInit)
	id.Send(selAutorelease)
	return id
}

// dictSetKV is the hot path — called ~12× per Update tick.

func dictSetKV(dict, key, val objc.ID) {
	dict.Send(selSetValueForKey, val, key)
}

// sendCmd is the shared implementation for all MPRemoteCommand handlers.

func sendCmd(ct CommandType) int32 {
	_darwinCtrlMu.Lock()
	c := _darwinCtrl
	_darwinCtrlMu.Unlock()
	if c == nil {
		return cmdHandlerCommandFailed
	}
	select {
	case c.cmdChan <- Command{Type: ct}:
	default:
	}
	return cmdHandlerSuccess
}
