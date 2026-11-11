//go:build darwin

package cmd

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestDisabledOSMediaSkipsObjCSetup(t *testing.T) {
	if objc.GetClass("NeoVioletCommandHandler") != 0 {
		t.Fatal("media integration was initialized before the test")
	}
	if err := runWithOSMedia(false, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if objc.GetClass("NeoVioletCommandHandler") != 0 {
		t.Fatal("disabled system integration initialized the ObjC runtime")
	}
}
