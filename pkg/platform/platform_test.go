package platform

import (
	"runtime"
	"testing"
)

func TestGetExecutableExtension(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{"windows", ".exe"},
		{"Windows", ".exe"},
		{"WINDOWS", ".exe"},
		{"macos", ".app"},
		{"osx", ".app"},
		{"linux", ""},
		{"Linux", ""},
		{"android", ".apk"},
		{"ios", ""},
		{"webgl", ""},
		{"unknown", ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := GetExecutableExtension(tt.target)
			if runtime.GOOS == "windows" && tt.target == "unknown" {
				if got != ".exe" {
					t.Errorf("GetExecutableExtension(%q) on Windows = %v, want .exe", tt.target, got)
				}
			} else if got != tt.want {
				t.Errorf("GetExecutableExtension(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestGetCurrentPlatform(t *testing.T) {
	platform := GetCurrentPlatform()
	
	switch runtime.GOOS {
	case "darwin":
		if platform != "macos" {
			t.Errorf("GetCurrentPlatform() on darwin = %v, want macos", platform)
		}
	case "windows":
		if platform != "windows" {
			t.Errorf("GetCurrentPlatform() on windows = %v, want windows", platform)
		}
	case "linux":
		if platform != "linux" {
			t.Errorf("GetCurrentPlatform() on linux = %v, want linux", platform)
		}
	default:
		if platform != runtime.GOOS {
			t.Errorf("GetCurrentPlatform() = %v, want %v", platform, runtime.GOOS)
		}
	}
}

func TestGetArchitecture(t *testing.T) {
	arch := GetArchitecture()
	
	switch runtime.GOARCH {
	case "amd64":
		if arch != "x64" {
			t.Errorf("GetArchitecture() on amd64 = %v, want x64", arch)
		}
	case "arm64":
		if arch != "arm64" {
			t.Errorf("GetArchitecture() on arm64 = %v, want arm64", arch)
		}
	case "386":
		if arch != "x86" {
			t.Errorf("GetArchitecture() on 386 = %v, want x86", arch)
		}
	default:
		if arch != runtime.GOARCH {
			t.Errorf("GetArchitecture() = %v, want %v", arch, runtime.GOARCH)
		}
	}
}

func TestIsUnixLike(t *testing.T) {
	isUnix := IsUnixLike()
	
	if runtime.GOOS == "windows" {
		if isUnix {
			t.Error("IsUnixLike() on Windows = true, want false")
		}
	} else {
		if !isUnix {
			t.Errorf("IsUnixLike() on %v = false, want true", runtime.GOOS)
		}
	}
}

func TestGetPlatformSpecificPath(t *testing.T) {
	tests := []struct {
		name  string
		paths map[string]string
		want  string
	}{
		{
			name: "Platform specific path exists",
			paths: map[string]string{
				"macos":   "/Applications/Unity",
				"windows": "C:\\Program Files\\Unity",
				"linux":   "/opt/unity",
			},
			want: func() string {
				platform := GetCurrentPlatform()
				paths := map[string]string{
					"macos":   "/Applications/Unity",
					"windows": "C:\\Program Files\\Unity",
					"linux":   "/opt/unity",
				}
				if path, ok := paths[platform]; ok {
					return path
				}
				return ""
			}(),
		},
		{
			name: "Use default path",
			paths: map[string]string{
				"default": "/usr/local/bin/unity",
			},
			want: "/usr/local/bin/unity",
		},
		{
			name: "Platform specific overrides default",
			paths: map[string]string{
				"default": "/usr/local/bin/unity",
				"macos":   "/Applications/Unity",
				"windows": "C:\\Program Files\\Unity",
				"linux":   "/opt/unity",
			},
			want: func() string {
				platform := GetCurrentPlatform()
				paths := map[string]string{
					"macos":   "/Applications/Unity",
					"windows": "C:\\Program Files\\Unity",
					"linux":   "/opt/unity",
				}
				if path, ok := paths[platform]; ok {
					return path
				}
				return "/usr/local/bin/unity"
			}(),
		},
		{
			name:  "No matching path",
			paths: map[string]string{},
			want:  "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPlatformSpecificPath(tt.paths)
			if got != tt.want {
				t.Errorf("GetPlatformSpecificPath() = %v, want %v", got, tt.want)
			}
		})
	}
}