package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// tokenEnvVar names the environment variable the bearer token is read from.
// There is no token flag: an argument shows in the process list.
const tokenEnvVar = "EUSOV_SERVER_TOKEN" //nolint:gosec // the variable's name, not a credential

// maxTokenFileSize caps what --token-file reads: a token is a line, so a file
// larger than this is not a token file.
const maxTokenFileSize = 64 << 10

// readToken returns the bearer token and whether one is configured: from the
// environment variable tokenEnvVar or from the file at tokenFile, never both.
// The white space around the token is trimmed. A source that is configured but
// holds no token (a variable set to the empty string, an empty file) is an
// error, as is a file that cannot be read: it is never read as "no token". No
// error quotes the token or the file's content.
func readToken(lookupEnv func(string) (string, bool), tokenFile string) (string, bool, error) {
	value, fromEnv := lookupEnv(tokenEnvVar)
	fromFile := tokenFile != ""

	switch {
	case fromEnv && fromFile:
		return "", false, fmt.Errorf("both %s and --token-file are set; configure the token in one of them", tokenEnvVar)
	case fromEnv:
		token := strings.TrimSpace(value)
		if token == "" {
			return "", false, fmt.Errorf("%s is set but holds no token", tokenEnvVar)
		}

		return token, true, nil
	case fromFile:
		token, err := readTokenFile(tokenFile)
		if err != nil {
			return "", false, fmt.Errorf("--token-file: %w", err)
		}

		return token, true, nil
	default:
		return "", false, nil
	}
}

// errEmptyTokenFile is returned for a token file that holds only white space.
var errEmptyTokenFile = errors.New("the token file holds no token")

// readTokenFile reads the token from the file at path, trimmed. The error of a
// file that cannot be opened or read is the operating system's, which names the
// path and never the content.
func readTokenFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // the operator names the token file
	if err != nil {
		return "", err
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxTokenFileSize+1))
	if err != nil {
		return "", err
	}

	if len(data) > maxTokenFileSize {
		return "", fmt.Errorf("the token file is larger than %d bytes", maxTokenFileSize)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errEmptyTokenFile
	}

	return token, nil
}
