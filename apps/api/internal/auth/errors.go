package auth

import "errors"

var (
	// ErrNotFound reports a missing user or session row.
	ErrNotFound = errors.New("auth: record not found")
	// ErrInvalidCredentials is returned for both unknown usernames and wrong
	// passwords so callers cannot enumerate accounts.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrUsernameTaken reports a duplicate username.
	ErrUsernameTaken = errors.New("auth: username already taken")
	// ErrRootAlreadyInitialized reports that the installation already has its
	// single ROOT identity; repeated bootstrap must be refused safely.
	ErrRootAlreadyInitialized = errors.New("auth: root already initialized")
	// ErrRoleAlreadyGranted reports a duplicate platform role grant.
	ErrRoleAlreadyGranted = errors.New("auth: platform role already granted")
)
