// Command windows-resources generates content-addressed Windows resource sources.
// Make compiles them with LLVM or windres and links them into each executable.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	component := flag.String("component", "", "tui, gui or apecli")
	arch := flag.String("arch", "", "386, amd64 or arm64")
	debug := flag.Bool("debug", false, "mark a debug build")
	flag.Parse()
	path, err := generate(*component, *arch, *debug, time.Now().Year())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Windows resources:", err)
		os.Exit(1)
	}
	fmt.Println(filepath.ToSlash(path))
}

func generate(component, arch string, debug bool, year int) (string, error) {
	switch arch {
	case "386", "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture %q", arch)
	}
	version := os.Getenv("WINDOWS_VERSION")
	if component == "apecli" {
		manifest, err := os.ReadFile("tools/apecli/Cargo.toml")
		if err != nil {
			return "", err
		}
		version, err = packageVersion(manifest)
		if err != nil {
			return "", err
		}
	}
	resource, err := renderResource(component, version, debug, year)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	if component != "apecli" {
		iconRC, err := os.ReadFile("assets/icons/neoviolet.rc")
		if err != nil {
			return "", err
		}
		icon, err := os.ReadFile("assets/icons/neoviolet.ico")
		if err != nil {
			return "", err
		}
		resource += "\n" + string(iconRC)
		hash.Write(icon)
	}
	hash.Write([]byte(resource))
	name := fmt.Sprintf("%s_%s_%x.rc", component, arch, hash.Sum(nil)[:16])
	path := filepath.Join(".build", "windows", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	// Component and arch are whitelisted; the remaining filename is a hex digest.
	if err := os.WriteFile(path, []byte(resource), 0o600); err != nil { // #nosec G703 -- no input can introduce a path separator.
		return "", err
	}
	return path, nil
}

var (
	versionText = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]*$`)
	versionCore = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)([-+].*)?$`)
)

func renderResource(component, version string, debug bool, year int) (string, error) {
	if !versionText.MatchString(version) {
		return "", fmt.Errorf("invalid version %q: use ASCII letters, digits, dots, hyphens, underscores or plus signs", version)
	}
	numeric := "0,0,0,0"
	flags := 2 // VS_FF_PRERELEASE for development builds without a version core.
	if parts := versionCore.FindStringSubmatch(version); parts != nil {
		for i := 1; i <= 3; i++ {
			n, err := strconv.ParseUint(parts[i], 10, 16)
			if err != nil {
				return "", fmt.Errorf("version %q: each numeric part must be between 0 and 65535", version)
			}
			parts[i] = strconv.FormatUint(n, 10)
		}
		numeric = strings.Join(parts[1:4], ",") + ",0"
		flags = 0
		if strings.HasPrefix(parts[4], "-") {
			flags = 2
		}
	}
	if debug {
		flags |= 1 // VS_FF_DEBUG.
	}
	product, name, description := "NeoViolet", "neoviolet", "NeoViolet terminal music player"
	switch component {
	case "tui":
	case "gui":
		name, description = "neoviolet-gui", "NeoViolet graphical frontend"
	case "apecli":
		product, name, description = "apecli", "apecli", "Monkey's Audio decoder command-line tool"
	default:
		return "", fmt.Errorf("unknown component %q", component)
	}
	// Keep the project's first copyright year from LICENSE and extend it to
	// the build year. The caller supplies the year so generation is testable.
	years := "2026"
	if year > 2026 {
		years += "-" + strconv.Itoa(year)
	}
	// Numeric constants keep this source compatible with llvm-rc -no-preprocess
	// without requiring Windows SDK headers. 0409/04b0 is US English/Unicode.
	return fmt.Sprintf(`LANGUAGE 9, 1
1 VERSIONINFO
FILEVERSION %s
PRODUCTVERSION %s
FILEFLAGSMASK 0x3f
FILEFLAGS %d
FILEOS 0x40004
FILETYPE 1
FILESUBTYPE 0
BEGIN
    BLOCK "StringFileInfo"
    BEGIN
        BLOCK "040904b0"
        BEGIN
            VALUE "CompanyName", "AuroraStudio\0"
            VALUE "FileDescription", "%s\0"
            VALUE "FileVersion", "%s\0"
            VALUE "InternalName", "%s\0"
            VALUE "LegalCopyright", "Copyright (c) %s AuroraStudio\0"
            VALUE "OriginalFilename", "%s.exe\0"
            VALUE "ProductName", "%s\0"
            VALUE "ProductVersion", "%s\0"
        END
    END
    BLOCK "VarFileInfo"
    BEGIN
        VALUE "Translation", 0x0409, 1200
    END
END
`, numeric, numeric, flags, description, version, name, years, name, product, version), nil
}

// packageVersion reads the literal version in [package], not a dependency's
// version. The standalone apecli manifest does not inherit a workspace version.
func packageVersion(data []byte) (string, error) {
	inPackage := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inPackage = line == "[package]"
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if inPackage && ok && strings.TrimSpace(key) == "version" {
			version, err := strconv.Unquote(strings.TrimSpace(value))
			if err != nil {
				return "", fmt.Errorf("read Cargo package version: %w", err)
			}
			return version, nil
		}
	}
	return "", fmt.Errorf("no literal [package] version in Cargo manifest")
}
