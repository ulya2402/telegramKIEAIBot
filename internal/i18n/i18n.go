package i18n

import (
	"encoding/json"
	"strings"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type Localizer struct {
	translations map[string]map[string]string
	mu           sync.RWMutex
	defaultLang  string
}

func NewLocalizer(defaultLang string) *Localizer {
	l := &Localizer{
		translations: make(map[string]map[string]string),
		defaultLang:  defaultLang,
	}
	l.loadTranslations()
	return l
}

func (l *Localizer) loadTranslations() {
	files, err := filepath.Glob("locales/*.json")
	if err != nil {
		log.Printf("Error finding locale files: %v\n", err)
		return
	}

	for _, file := range files {
		langCode := strings.TrimSuffix(filepath.Base(file), ".json")
		content, err := os.ReadFile(file)
		if err != nil {
			log.Printf("Error reading locale file %s: %v\n", file, err)
			continue
		}

		var data map[string]string
		if err := json.Unmarshal(content, &data); err != nil {
			log.Printf("Error parsing locale file %s: %v\n", file, err)
			continue
		}

		l.mu.Lock()
		l.translations[langCode] = data
		l.mu.Unlock()
		log.Printf("Loaded language: %s\n", langCode)
	}
}

func (l *Localizer) Get(lang, key string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if lang == "" {
		lang = l.defaultLang
	}

	if trans, ok := l.translations[lang]; ok {
		if val, ok := trans[key]; ok {
			return val
		}
	}

	if trans, ok := l.translations[l.defaultLang]; ok {
		if val, ok := trans[key]; ok {
			return val
		}
	}

	return key
}

