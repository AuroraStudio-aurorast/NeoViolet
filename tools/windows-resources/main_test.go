package main

import (
	"os"
	"strings"
	"testing"
)

func TestVersionInfo(t *testing.T) {
	for _, tt := range []struct {
		version string
		numeric string
		flags   string
	}{
		{"v1.2.3", "1,2,3,0", "0"},
		{"1.2.3+build.7", "1,2,3,0", "0"},
		{"v1.2.3-rc.1", "1,2,3,0", "2"},
		{"v1.2.3-5-gabcdef", "1,2,3,0", "2"},
		{"v1.2.3-dirty", "1,2,3,0", "2"},
		{"dev-r123456-abcdef", "0,0,0,0", "2"},
		{"abcdef0", "0,0,0,0", "2"},
		{"dev", "0,0,0,0", "2"},
		{"v65535.0.65535", "65535,0,65535,0", "0"},
	} {
		t.Run(tt.version, func(t *testing.T) {
			resource, err := renderResource("tui", tt.version, false, 2026)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"FILEVERSION " + tt.numeric,
				"PRODUCTVERSION " + tt.numeric,
				"FILEFLAGS " + tt.flags + "\n",
				`VALUE "FileVersion", "` + tt.version + `\0"`,
				`VALUE "ProductVersion", "` + tt.version + `\0"`,
			} {
				if !strings.Contains(resource, want) {
					t.Errorf("resource does not contain %q:\n%s", want, resource)
				}
			}
		})
	}
}

func TestRejectInvalidVersion(t *testing.T) {
	for _, version := range []string{
		"", "v65536.0.0", "1.999999999999999999999.0", "1.0.65536",
		"v1.2.3\n1 ICON bad", `1.2.3"`, `1.2.3\0`, "$(whoami)", "1.2.3 café",
	} {
		t.Run(version, func(t *testing.T) {
			if _, err := renderResource("tui", version, false, 2026); err == nil {
				t.Fatalf("accepted invalid version %q", version)
			}
		})
	}
}

func TestComponentIdentity(t *testing.T) {
	for _, tt := range []struct {
		component string
		product   string
		name      string
	}{
		{"tui", "NeoViolet", "neoviolet"},
		{"gui", "NeoViolet", "neoviolet-gui"},
		{"apecli", "apecli", "apecli"},
	} {
		t.Run(tt.component, func(t *testing.T) {
			resource, err := renderResource(tt.component, "0.1.0", true, 2026)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				`VALUE "ProductName", "` + tt.product + `\0"`,
				`VALUE "OriginalFilename", "` + tt.name + `.exe\0"`,
				"FILEFLAGS 1\n",
			} {
				if !strings.Contains(resource, want) {
					t.Errorf("resource missing %q", want)
				}
			}
			if tt.component == "apecli" && strings.Contains(resource, "NeoViolet") {
				t.Fatal("apecli metadata contains NeoViolet branding")
			}
		})
	}
	if _, err := renderResource("unknown", "1.0.0", false, 2026); err == nil {
		t.Fatal("accepted unknown component")
	}
	resource, err := renderResource("tui", "dev", true, 2026)
	if err != nil || !strings.Contains(resource, "FILEFLAGS 3\n") {
		t.Fatalf("debug development build must set both flags: %s, %v", resource, err)
	}
}

func TestPackageVersion(t *testing.T) {
	content := "[workspace.package]\nversion = \"9.0.0\"\n[package]\nname = \"apecli\"\nversion = \"0.2.1\"\n[dependencies]\nversion = \"8.0.0\"\n"
	got, err := packageVersion([]byte(content))
	if err != nil || got != "0.2.1" {
		t.Fatalf("packageVersion = %q, %v; want 0.2.1", got, err)
	}
	if _, err := packageVersion([]byte("[dependencies]\nversion = \"8.0.0\"\n")); err == nil {
		t.Fatal("accepted manifest without package version")
	}
}

func TestResourceCacheInputs(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, dir := range []string{"assets/icons", "tools/apecli"} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("assets/icons/neoviolet.rc", "1 ICON \"assets/icons/neoviolet.ico\"\n")
	write("assets/icons/neoviolet.ico", "icon A")
	write("tools/apecli/Cargo.toml", "[package]\nversion = \"0.1.0\"\n")
	resource := func(component, arch string, debug bool) string {
		t.Helper()
		path, err := generate(component, arch, debug, 2026)
		if err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Setenv("WINDOWS_VERSION", "v1.2.3")
	first := resource("tui", "amd64", false)
	apecli := resource("apecli", "amd64", false)
	for _, component := range []string{"tui", "gui", "apecli"} {
		current := resource(component, "amd64", false)
		nextYear, err := generate(component, "amd64", false, 2027)
		if err != nil {
			t.Fatal(err)
		}
		if nextYear == current {
			t.Fatalf("%s: copyright year change reused the resource path", component)
		}
	}
	t.Setenv("WINDOWS_VERSION", "v2.0.0")
	if resource("tui", "amd64", false) == first {
		t.Fatal("version change reused the resource path")
	}
	if resource("apecli", "amd64", false) != apecli {
		t.Fatal("suite version affected apecli")
	}
	t.Setenv("WINDOWS_VERSION", "v1.2.3")
	if resource("tui", "amd64", false) != first {
		t.Fatal("version rollback did not restore the original resource path")
	}
	if resource("tui", "amd64", true) == first || resource("tui", "arm64", false) == first {
		t.Fatal("debug profile or architecture reused the release resource path")
	}
	write("assets/icons/neoviolet.ico", "icon B")
	if resource("tui", "amd64", false) == first {
		t.Fatal("icon change reused the resource path")
	}
	if resource("apecli", "amd64", false) != apecli {
		t.Fatal("NeoViolet icon affected apecli")
	}
	write("tools/apecli/Cargo.toml", "[package]\nversion = \"0.2.0\"\n")
	if resource("apecli", "amd64", false) == apecli {
		t.Fatal("component version change reused the resource path")
	}
	if _, err := generate("tui", "unknown", false, 2026); err == nil {
		t.Fatal("accepted unsupported architecture")
	}
}

func TestCopyrightYears(t *testing.T) {
	for _, component := range []string{"tui", "gui", "apecli"} {
		for _, tt := range []struct {
			year int
			want string
		}{
			{2026, "2026"},
			{2027, "2026-2027"},
			{2030, "2026-2030"},
		} {
			resource, err := renderResource(component, "1.0.0", false, tt.year)
			if err != nil {
				t.Fatal(err)
			}
			want := `VALUE "LegalCopyright", "Copyright (c) ` + tt.want + ` AuroraStudio\0"`
			if !strings.Contains(resource, want) {
				t.Errorf("%s year %d: resource missing %q", component, tt.year, want)
			}
		}
	}
}
