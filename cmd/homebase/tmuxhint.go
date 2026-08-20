package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/yanghanqing/homebase/internal/tmux"
)

func requireTmux() error {
	_, err := tmux.LocalBinary()
	if err != nil {
		printNoTmux(os.Stderr)
		return err
	}
	return nil
}

// printNoTmux matches the installer hint. Keep the search-path comment in
// install.sh pointed at internal/tmux.LocalBinary, and this text pointed at
// the same block in install.sh.
func printNoTmux(w io.Writer) {
	fmt.Fprintln(w, "homebase is installed, but tmux was not found.")
	fmt.Fprintln(w, "Homebase is a tmux frontend — install tmux, then start it.")
	fmt.Fprintln(w)
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			fmt.Fprintln(w, "  macOS (Homebrew):     brew install tmux")
		} else {
			fmt.Fprintln(w, "  macOS: install Homebrew (https://brew.sh), then:  brew install tmux")
			fmt.Fprintln(w, "  or:    https://github.com/tmux/tmux/wiki/Installing")
		}
		fmt.Fprintln(w, "  other systems: Debian/Ubuntu apt, Fedora dnf, Arch pacman")
	case "linux":
		switch linuxID() {
		case "debian", "ubuntu":
			fmt.Fprintln(w, "  Debian / Ubuntu:      sudo apt install tmux")
			fmt.Fprintln(w, "  other systems: macOS brew, Fedora dnf, Arch pacman")
		case "fedora":
			fmt.Fprintln(w, "  Fedora:               sudo dnf install tmux")
			fmt.Fprintln(w, "  other systems: macOS brew, Debian/Ubuntu apt, Arch pacman")
		case "arch", "manjaro":
			fmt.Fprintln(w, "  Arch:                 sudo pacman -S tmux")
			fmt.Fprintln(w, "  other systems: macOS brew, Debian/Ubuntu apt, Fedora dnf")
		default:
			fmt.Fprintln(w, "  Debian / Ubuntu:      sudo apt install tmux")
			fmt.Fprintln(w, "  Fedora:               sudo dnf install tmux")
			fmt.Fprintln(w, "  Arch:                 sudo pacman -S tmux")
			fmt.Fprintln(w, "  other systems: macOS brew install tmux")
		}
	default:
		fmt.Fprintln(w, "  macOS (Homebrew):     brew install tmux")
		fmt.Fprintln(w, "  Debian / Ubuntu:      sudo apt install tmux")
		fmt.Fprintln(w, "  Fedora:               sudo dnf install tmux")
		fmt.Fprintln(w, "  Arch:                 sudo pacman -S tmux")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Then:  homebase start")
}

func linuxID() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			return strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		}
	}
	return ""
}
