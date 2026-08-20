package main

import (
	"fmt"
	"runtime/debug"
)

// version is set by GoReleaser via -ldflags "-X main.version={{.Version}}".
// A `go build` without that flag must still print something other than empty.
var version = ""

func runVersion(args []string) int {
	fmt.Println(versionString())
	return 0
}

func versionString() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			rev := s.Value
			if len(rev) > 7 {
				rev = rev[:7]
			}
			return "dev (" + rev + ")"
		}
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return "dev"
}
