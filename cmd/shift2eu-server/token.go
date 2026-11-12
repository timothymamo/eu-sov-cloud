package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// tokenEnvVar names the environment variable the bearer token is read from.
// There is no token flag: an argument shows in the process list.
const tokenEnvVar = "SHIFT2EU_SERVER_TOKEN" //nolint:gosec // the variable's name, not a credential

// maxTokenFileSize caps what --token-file reads: a token is a line, so a file
// larger than this is not a token file.
const maxTokenFileSize = 64 << 10

// minTokenLength is the shortest bearer token the server accepts, in
// characters after trimming (bytes: a bearer token is ASCII, RFC 6750
// token68): a shorter one is within reach of a client that guesses (ADR 0019).
const minTokenLength = 32

// tokenConfig is the bearer token the server is configured with.
type tokenConfig struct {
	// token is the token, trimmed; set reports whether one is configured.
	token string
	set   bool
	// warning, when not empty, is a startup warning about the token's source,
	// which never quotes the token.
	warning string
}

// readToken returns the bearer token, if one is configured: from the
// environment variable tokenEnvVar or from the file at tokenFile, never both.
// The white space around the token is trimmed. A source that is configured but
// holds no token (a variable set to the empty string, an empty file), or a token
// shorter than minTokenLength, is an error, as is a file that cannot be read: it
// is never read as "no token". A token file that the group or other users can
// access earns a warning, on systems with those permission bits. No error or
// warning quotes the token or the file's content.
func readToken(lookupEnv func(string) (string, bool), tokenFile string) (tokenConfig, error) {
	value, fromEnv := lookupEnv(tokenEnvVar)
	fromFile := tokenFile != ""

	switch {
	case fromEnv && fromFile:
		return tokenConfig{}, fmt.Errorf("both %s and --token-file are set; configure the token in one of them", tokenEnvVar)
	case fromEnv:
		token := strings.TrimSpace(value)
		if token == "" {
			return tokenConfig{}, fmt.Errorf("%s is set but holds no token", tokenEnvVar)
		}

		if err := checkTokenLength(token); err != nil {
			return tokenConfig{}, fmt.Errorf("%s: %w", tokenEnvVar, err)
		}

		return tokenConfig{token: token, set: true}, nil
	case fromFile:
		token, warning, err := readTokenFile(tokenFile)
		if err == nil {
			err = checkTokenLength(token)
		}

		if err != nil {
			return tokenConfig{}, fmt.Errorf("--token-file: %w", err)
		}

		return tokenConfig{token: token, set: true, warning: warning}, nil
	default:
		return tokenConfig{}, nil
	}
}

// checkTokenLength refuses a token shorter than minTokenLength, without
// quoting it.
func checkTokenLength(token string) error {
	// len counts bytes, which equals characters here: httpapi.WithBearerToken
	// refuses any token that is not an RFC 6750 token68 (printable ASCII), so
	// a non-ASCII token is a startup error anyway.
	if len(token) < minTokenLength {
		return fmt.Errorf("the token is shorter than %d characters", minTokenLength)
	}

	return nil
}

// errEmptyTokenFile is returned for a token file that holds only white space.
var errEmptyTokenFile = errors.New("the token file holds no token")

// readTokenFile reads the token from the file at path, trimmed, and returns a
// warning naming the path when the file's group or other permission bits are
// set (never on Windows, which has none). The error of a file that cannot be
// opened or read is the operating system's, which names the path and never the
// content.
func readTokenFile(path string) (token, warning string, err error) {
	f, err := os.Open(path) //nolint:gosec // the operator names the token file
	if err != nil {
		return "", "", err
	}

	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", "", err
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		warning = fmt.Sprintf("the token file %q is accessible to other users (mode %v); restrict it to its owner (chmod 600)",
			path, info.Mode().Perm())
	}

	data, err := io.ReadAll(io.LimitReader(f, maxTokenFileSize+1))
	if err != nil {
		return "", "", err
	}

	if len(data) > maxTokenFileSize {
		return "", "", fmt.Errorf("the token file is larger than %d bytes", maxTokenFileSize)
	}

	token = strings.TrimSpace(string(data))
	if token == "" {
		return "", "", errEmptyTokenFile
	}

	return token, warning, nil
}
