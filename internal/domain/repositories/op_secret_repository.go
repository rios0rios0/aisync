package repositories

// OpSecretRepository fetches secret material from a 1Password vault
// via the `op` CLI. The contract is intentionally narrow: aisync only
// needs to retrieve a single age identity by item name, so a future
// adapter (op-connect, REST API, biometric vault, etc.) can satisfy it
// without exposing unrelated 1Password operations to the domain layer.
type OpSecretRepository interface {
	// GetIdentity returns the AGE-SECRET-KEY-1... value stored in the
	// "private key" field of the named item, looked up inside the
	// given vault. The vault may be empty, in which case the
	// adapter searches all readable vaults for the active 1Password
	// session.
	GetIdentity(vault, item string) (string, error)
}
