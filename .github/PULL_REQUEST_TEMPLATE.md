## What this changes

<!-- One or two sentences. What behavior is different after this PR? -->

## Why

<!-- The problem being solved. Link an issue if there is one. -->

## Checklist

- [ ] `gofmt -l .` prints nothing, `go vet ./...` is clean
- [ ] `go test ./...` passes (and `go test -v ./internal/tmux/` says PASS, not
      SKIP, if this touches `internal/tmux`)
- [ ] `go mod tidy` leaves `go.mod` and `go.sum` unchanged
- [ ] README.md / README.zh.md / SECURITY.md updated if this changes anything
      they describe
- [ ] Commit messages are English with an imperative subject

## Anything reviewers should look at closely

<!-- Listen-address policy, tmux argv, PTY lifetime, WebSocket framing, resize,
     reconnect, and the pairing flow are the fragile parts. Say so if you
     touched one. -->
