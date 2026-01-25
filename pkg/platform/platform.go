package platform

import (
	"runtime"
	"strings"
)

func GetExecutableExtension(target string) string {
	switch strings.ToLower(target) {
	case "windows":
		return ".exe"
	case "macos", "osx":
		return ".app"
	case "linux":
		return ""
	case "android":
		return ".apk"
	case "ios":
		return ""
	case "webgl":
		return ""
	default:
		if runtime.GOOS == "windows" {
			return ".exe"
		}
		return ""
	}
}

func GetCurrentPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}

func GetArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}

func IsUnixLike() bool {
	return runtime.GOOS != "windows"
}

func GetPlatformSpecificPath(paths map[string]string) string {
	platform := GetCurrentPlatform()
	if path, ok := paths[platform]; ok {
		return path
	}

	if defaultPath, ok := paths["default"]; ok {
		return defaultPath
	}

	return ""
}
