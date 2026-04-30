package repositories

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	logger "github.com/sirupsen/logrus"
)

// opPrivateKeyField is the canonical label of the field that stores the
// AGE-SECRET-KEY-1... value on the 1Password item. The label is fixed
// (rather than configurable) by design: aisync defines a small contract
// the user provisions once, and freezing the field name keeps the lookup
// path the same on every device.
const opPrivateKeyField = "private key"

// OpCLISecretRepository implements [repositories.OpSecretRepository] by
// shelling out to the official `op` binary. The session/auth model is
// delegated entirely to `op` itself — biometric prompt, SSH-agent, or
// service-account token are all handled by the CLI and aisync never
// touches the master password.
type OpCLISecretRepository struct {
	binary string
}

// NewOpCLISecretRepository creates a new adapter using the `op` binary
// resolved from PATH. The binary path is captured at construction time
// so a misconfigured PATH surfaces immediately rather than on first use.
func NewOpCLISecretRepository() *OpCLISecretRepository {
	return &OpCLISecretRepository{binary: "op"}
}

// GetIdentity looks up the item by name in the configured vault and
// returns the trimmed contents of its "private key" field. The vault
// argument may be empty, in which case `op` searches all readable
// vaults for the active session.
func (r *OpCLISecretRepository) GetIdentity(vault, item string) (string, error) {
	if item == "" {
		return "", errors.New("op item name is required")
	}

	args := []string{
		"item", "get", item,
		"--fields", "label=" + opPrivateKeyField,
		"--reveal",
	}
	if vault != "" {
		args = append(args, "--vault", vault)
	}

	logger.Debugf("running %s %s", r.binary, strings.Join(args, " "))

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(r.binary, args...) //nolint:gosec // args built from validated config; binary is the official op CLI
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return "", fmt.Errorf(
				"1Password CLI not available (install `op` and run `op signin`): %w", err,
			)
		}
		return "", fmt.Errorf(
			"op item get failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()),
		)
	}

	identity := strings.TrimRight(stdout.String(), "\r\n")
	if identity == "" {
		return "", fmt.Errorf(
			"item %q in vault %q has empty %q field", item, vault, opPrivateKeyField,
		)
	}

	return identity, nil
}
