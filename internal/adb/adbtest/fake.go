// Package adbtest provides a fake adb runner for tests.
package adbtest

import (
	"errors"
	"strings"
)

// Response is a canned adb result.
type Response struct {
	Stdout, Stderr string
	Err            error
}

// Fake answers adb calls from Responses, keyed by the space-joined args.
type Fake struct {
	Responses map[string]Response
	Calls     []string
}

// ErrUnexpected is returned for calls without a canned response.
var ErrUnexpected = errors.New("unexpected adb call")

func (f *Fake) Run(args ...string) (string, string, error) {
	key := strings.Join(args, " ")
	f.Calls = append(f.Calls, key)
	r, ok := f.Responses[key]
	if !ok {
		return "", "", ErrUnexpected
	}
	return r.Stdout, r.Stderr, r.Err
}

func (f *Fake) Passthrough(args ...string) error {
	_, _, err := f.Run(args...)
	return err
}
