package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

//go:embed en.json es.json
var catalogs embed.FS

// Supported language codes.
const (
	English = "en"
	Spanish = "es"
)

var (
	mu       sync.RWMutex
	current  = English
	messages = map[string]map[string]string{}
)

func init() {
	for _, lang := range Available() {
		data, err := catalogs.ReadFile(lang + ".json")
		if err != nil {
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		messages[lang] = m
	}
}

// Available returns the supported language codes.
func Available() []string {
	return []string{English, Spanish}
}

// SetLanguage switches the active language. Unknown codes are ignored.
func SetLanguage(lang string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := messages[lang]; ok {
		current = lang
	}
}

// Current returns the active language code.
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T returns the translation of key for the active language. It falls back to
// English and finally to the key itself when no translation exists.
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if m, ok := messages[current]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := messages[English]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// TFor returns the translation of key for a specific language, falling back to
// English and finally to the key itself. It is used when the language to render
// is not the active one (e.g. the startup language chooser).
func TFor(lang, key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if m, ok := messages[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := messages[English]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// Error carries a translation key plus an optional technical detail. Libraries
// return it so the UI can render it in the active language without hardcoding
// user-facing text.
type Error struct {
	Key    string
	Detail string
}

// NewError builds an Error for the given key and optional detail.
func NewError(key, detail string) *Error {
	return &Error{Key: key, Detail: detail}
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return e.Key + ": " + e.Detail
	}
	return e.Key
}

// Message renders err in the active language. For *Error it translates Key and
// appends Detail; any other error is returned as-is.
func Message(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		msg := T(e.Key)
		if e.Detail != "" {
			return fmt.Sprintf("%s (%s)", msg, e.Detail)
		}
		return msg
	}
	return err.Error()
}
