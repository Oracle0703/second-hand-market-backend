//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func hasControlledPasswordFileMode(os.FileMode) bool {
	// Go file modes do not expose Windows ACLs, so secret files fail closed.
	return false
}

func readHiddenAdminPassword() (password string, resultErr error) {
	console := windows.Handle(os.Stdin.Fd())
	return readHiddenAdminPasswordFromConsole(
		os.Stdin,
		os.Stderr,
		func(mode *uint32) error {
			return windows.GetConsoleMode(console, mode)
		},
		func(mode uint32) error {
			return windows.SetConsoleMode(console, mode)
		},
	)
}

func readHiddenAdminPasswordFromConsole(
	input io.Reader,
	prompt io.Writer,
	getMode func(*uint32) error,
	setMode func(uint32) error,
) (password string, resultErr error) {
	var originalMode uint32
	if err := getMode(&originalMode); err != nil {
		return "", errors.New("ADMIN_PASSWORD_INPUT is unavailable")
	}
	if err := setMode(originalMode &^ windows.ENABLE_ECHO_INPUT); err != nil {
		return "", errors.New("ADMIN_PASSWORD_INPUT is unavailable")
	}
	defer func() {
		if err := setMode(originalMode); err != nil {
			password = ""
			resultErr = errors.New("ADMIN_PASSWORD_INPUT is unavailable")
		}
	}()

	_, _ = fmt.Fprint(prompt, "ADMIN_PASSWORD: ")
	data, overflow, err := readHiddenAdminPasswordLine(input)
	_, _ = fmt.Fprintln(prompt)
	if err != nil {
		return "", errors.New("ADMIN_PASSWORD_INPUT is unavailable")
	}
	defer clear(data)
	if overflow {
		return "", errors.New("ADMIN_PASSWORD_INPUT is invalid")
	}

	password, err = normalizeAdminPassword(data)
	if err != nil {
		return "", errors.New("ADMIN_PASSWORD_INPUT is invalid")
	}
	return password, nil
}

func readHiddenAdminPasswordLine(input io.Reader) ([]byte, bool, error) {
	data := make([]byte, 0, maxAdminPasswordBytes+2)
	overflow := false
	oneByte := make([]byte, 1)
	for {
		readCount, err := input.Read(oneByte)
		if readCount == 1 {
			if len(data) < maxAdminPasswordBytes+2 {
				data = append(data, oneByte[0])
			} else {
				overflow = true
			}
			if oneByte[0] == '\n' {
				return data, overflow, nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return data, overflow, nil
			}
			clear(data)
			return nil, false, err
		}
		if readCount == 0 {
			clear(data)
			return nil, false, io.ErrNoProgress
		}
	}
}
