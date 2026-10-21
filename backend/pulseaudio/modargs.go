package pulseaudio

import (
	"errors"
	"fmt"
	"strings"
)

// parseModuleArgs splits a module argument string as PulseAudio's pa_modargs
// does: whitespace-separated key=value pairs, each value bare, 'single' or
// "double" quoted, with backslash escapes.
func parseModuleArgs(s string) (map[string]string, error) {
	args := map[string]string{}
	r := argReader{rest: s}
	for r.skipSpace() {
		key, err := r.key()
		if err != nil {
			return nil, err
		}
		value, err := r.value()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		if _, dup := args[key]; dup {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		args[key] = value
	}
	return args, nil
}

// argReader walks a module argument string.
type argReader struct {
	rest string
}

const spaces = " \t\n\v\f\r"

// skipSpace moves to the next argument and reports whether there is one.
func (r *argReader) skipSpace() bool {
	r.rest = strings.TrimLeft(r.rest, spaces)
	return r.rest != ""
}

// key reads a key and the '=' after it.
func (r *argReader) key() (string, error) {
	n := strings.IndexAny(r.rest, "="+spaces)
	if n < 0 || r.rest[n] != '=' {
		word, _, _ := strings.Cut(r.rest, " ")
		return "", fmt.Errorf("key %q without value", word)
	}
	if n == 0 {
		return "", errors.New("value without key")
	}
	key := r.rest[:n]
	r.rest = r.rest[n+1:]
	return key, nil
}

// value reads a bare value, or a quoted one and its closing quote.
func (r *argReader) value() (string, error) {
	if r.rest == "" || (r.rest[0] != '\'' && r.rest[0] != '"') {
		return r.until(isSpace)
	}
	quote := r.rest[0]
	r.rest = r.rest[1:]
	v, err := r.until(func(c byte) bool { return c == quote })
	if err != nil {
		return "", err
	}
	if r.rest == "" {
		return "", errors.New("unterminated quote")
	}
	r.rest = r.rest[1:]
	return v, nil
}

// until reads up to the first unescaped byte matching stop, unescaping
// backslashes.
func (r *argReader) until(stop func(byte) bool) (string, error) {
	var v strings.Builder
	for r.rest != "" && !stop(r.rest[0]) {
		if r.rest[0] == '\\' {
			if len(r.rest) == 1 {
				return "", errors.New("trailing backslash")
			}
			r.rest = r.rest[1:]
		}
		v.WriteByte(r.rest[0])
		r.rest = r.rest[1:]
	}
	return v.String(), nil
}

func isSpace(c byte) bool {
	return strings.IndexByte(spaces, c) >= 0
}
