//go:build darwin

package mediactl

import (
	"github.com/ebitengine/purego/objc"
)

// ObjC convenience helpers

func nsString(s string) objc.ID {
	id := objc.ID(class_NSString).Send(sel_alloc).Send(sel_initWithUTF8String, s)
	id.Send(sel_autorelease)
	return id
}

func nsInt(v int32) objc.ID {
	return objc.ID(class_NSNumber).Send(sel_numberWithInt, v)
}

func nsDouble(v float64) objc.ID {
	return objc.ID(class_NSNumber).Send(sel_numberWithDouble, v)
}

func nsMutableDict() objc.ID {
	id := objc.ID(class_NSMutableDictionary).Send(sel_alloc).Send(sel_init)
	id.Send(sel_autorelease)
	return id
}

// dictSetKV is the hot path — called ~12× per Update tick.

func dictSetKV(dict, key, val objc.ID) {
	dict.Send(sel_setValueForKey, val, key)
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
