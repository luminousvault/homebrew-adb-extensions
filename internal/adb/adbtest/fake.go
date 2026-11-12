// Package adbtest provides a fake adb runner for tests.
package adbtest

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// Response is a canned adb result.
type Response struct {
	Stdout, Stderr string
	Err            error
	Delay          time.Duration // simulates a slow command
}

// Fake answers adb calls from Responses, keyed by the space-joined args.
// It is safe for concurrent use.
type Fake struct {
	Responses map[string]Response
	Calls     []string
	mu        sync.Mutex
}

// ErrUnexpected is returned for calls without a canned response.
var ErrUnexpected = errors.New("unexpected adb call")

func (f *Fake) Run(args ...string) (string, string, error) {
	key := strings.Join(args, " ")
	f.mu.Lock()
	f.Calls = append(f.Calls, key)
	r, ok := f.Responses[key]
	f.mu.Unlock()
	if !ok {
		return "", "", ErrUnexpected
	}
	time.Sleep(r.Delay)
	return r.Stdout, r.Stderr, r.Err
}

func (f *Fake) Passthrough(args ...string) error {
	_, _, err := f.Run(args...)
	return err
}
