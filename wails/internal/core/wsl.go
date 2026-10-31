package core

import "strings"

// WslDistro mirrors paths::WslDistro.
type WslDistro struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
	Version   uint8  `json:"version"`
	State     string `json:"state"`
}

// getWslInfo runs `wsl -l -v` and parses the distribution table. Returns an
// empty list when WSL is unavailable (mirrors paths::get_wsl_info, which
// never surfaces errors to the UI).
func getWslInfo() []WslDistro {
	output, ok := runWslCapture("-l", "-v")
	if !ok {
		return nil
	}
	return parseWslList(output)
}

// parseWslList parses `wsl -l -v` output.
//
// wsl.exe emits UTF-16LE when its stdout is redirected; the NUL bytes are
// stripped here (the same trick the Rust core relies on via its character
// filter). Lines look like:
//
//	NAME             STATE           VERSION
//
// * Ubuntu           Running         2
//
// The distribution name may contain spaces, so the last two whitespace-
// separated tokens are state and version and everything before them is the
// name.
func parseWslList(output string) []WslDistro {
	normalized := strings.ReplaceAll(output, "\x00", "")
	var distros []WslDistro
	first := true
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if first {
			// Header line ("NAME STATE VERSION").
			first = false
			continue
		}

		isDefault := strings.HasPrefix(line, "*")
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))

		var fields []string
		for _, field := range strings.Fields(line) {
			fields = append(fields, field)
		}
		if len(fields) < 3 {
			continue
		}
		version := uint8(2)
		if parsed := parseUint8(fields[len(fields)-1]); parsed > 0 {
			version = parsed
		}
		state := fields[len(fields)-2]
		name := strings.Join(fields[:len(fields)-2], " ")
		if name == "" {
			continue
		}
		distros = append(distros, WslDistro{
			Name:      name,
			IsDefault: isDefault,
			Version:   version,
			State:     state,
		})
	}
	return distros
}

func parseUint8(value string) uint8 {
	if value == "" || len(value) > 3 {
		return 0
	}
	result := uint8(0)
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		result = result*10 + uint8(r-'0')
	}
	return result
}

// getWslUsername resolves the Linux username of a distribution via
// `wsl -d <distro> whoami` (mirrors paths::get_wsl_username).
func getWslUsername(distro string) (string, bool) {
	output, ok := runWslCapture("-d", distro, "whoami")
	if !ok {
		return "", false
	}
	username := strings.TrimSpace(strings.ReplaceAll(output, "\x00", ""))
	if username == "" {
		return "", false
	}
	return username, true
}

// wslHermesPath builds the \\wsl$ path of a distribution's Hermes home
// (mirrors paths::build_wsl_path for the "hermes" key).
func wslHermesPath(distro, username string) string {
	return `\\wsl$\` + distro + `\home\` + username + `\.hermes`
}

// defaultHermesHomeWSL resolves the Hermes home through the WSL fallback
// chain used on Windows:
//
//  1. %LOCALAPPDATA%/hermes when it contains config.yaml
//  2. ~/.hermes when it contains config.yaml
//  3. the default WSL distribution's \\wsl$ home
//  4. ~/.hermes (final default)
func defaultHermesHomeWSL(home string) string {
	if local := localAppData(); local != "" {
		userPath := local + `\hermes`
		if fileExists(userPath + `\config.yaml`) {
			return userPath
		}
	}
	mainPath := home + `\.hermes`
	if fileExists(mainPath + `\config.yaml`) {
		return mainPath
	}
	for _, distro := range getWslInfo() {
		if !distro.IsDefault {
			continue
		}
		if username, ok := getWslUsername(distro.Name); ok {
			return wslHermesPath(distro.Name, username)
		}
		break
	}
	return mainPath
}
