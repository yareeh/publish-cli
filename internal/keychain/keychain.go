// Package keychain stores secrets in the macOS login keychain through the
// security(1) command.
package keychain

import (
	"errors"
	"os/exec"
	"strings"
)

const service = "publish-cli"

var ErrNotFound = errors.New("not found in keychain")

func Set(account, secret string) error {
	out, err := exec.Command("/usr/bin/security", "add-generic-password", "-U",
		"-s", service, "-a", account, "-w", secret).CombinedOutput()
	if err != nil {
		return errors.New("keychain: " + strings.TrimSpace(string(out)))
	}
	return nil
}

func Get(account string) (string, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password",
		"-s", service, "-a", account, "-w").Output()
	if err != nil {
		return "", ErrNotFound
	}
	return strings.TrimSpace(string(out)), nil
}

func Delete(account string) error {
	if err := exec.Command("/usr/bin/security", "delete-generic-password",
		"-s", service, "-a", account).Run(); err != nil {
		return ErrNotFound
	}
	return nil
}
