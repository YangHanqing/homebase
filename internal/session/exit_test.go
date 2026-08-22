package session

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDescribeExitCarriesTheStatus(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 7").Run()
	got := DescribeExit(err)
	if got.Code != 7 || got.Signal != "" || got.Err != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestDescribeExitNamesTheSignal(t *testing.T) {
	cmd := exec.Command("sh", "-c", "kill -TERM $$")
	got := DescribeExit(cmd.Run())
	if got.Signal != "terminated" {
		t.Fatalf("got %+v", got)
	}
	// A signalled process has no exit status of its own.
	if got.Code != -1 {
		t.Fatalf("code %d, want -1", got.Code)
	}
}

func TestDescribeExitSuccessAndOtherErrors(t *testing.T) {
	if got := DescribeExit(nil); got.Code != 0 || got.Signal != "" || got.Err != "" {
		t.Fatalf("nil: got %+v", got)
	}
	got := DescribeExit(errors.New("wait: no child processes"))
	if got.Code != -1 || got.Err != "wait: no child processes" {
		t.Fatalf("got %+v", got)
	}
}

func TestLogArgsOmitsEmptyFields(t *testing.T) {
	args := ExitDetail{Code: 0}.LogArgs()
	if len(args) != 2 || args[0] != "exit_code" {
		t.Fatalf("got %v", args)
	}
	args = ExitDetail{Code: -1, Signal: "killed", Err: "boom"}.LogArgs()
	if len(args) != 6 {
		t.Fatalf("got %v", args)
	}
}

func TestStderrSnippetIsBounded(t *testing.T) {
	got := StderrSnippet([]byte(strings.Repeat("x", 4096)))
	if len(got) != maxMessage {
		t.Fatalf("len %d, want %d", len(got), maxMessage)
	}
}
