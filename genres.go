package main

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

const unclassified = "Sin clasificar"

// Rule asocia un género propio con palabras clave (multi-idioma) que aparecen en los
// "subjects" de Open Library. Se editan en data/genre_rules.json.
type Rule struct {
	Name     string   `json:"name"`
	Keywords []string `json:"keywords"`
	Exclude  []string `json:"exclude,omitempty"`
}

type Rules []Rule

func loadRules(path string) (Rules, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rs Rules
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// Classify devuelve los géneros (en el orden de las reglas) que matchean algún subject.
func (rs Rules) Classify(subjects []string) []string {
	low := make([]string, len(subjects))
	for i, s := range subjects {
		low[i] = strings.ToLower(s)
	}
	var out []string
	for _, r := range rs {
	subj:
		for _, s := range low {
			for _, ex := range r.Exclude {
				if strings.Contains(s, ex) {
					continue subj
				}
			}
			for _, kw := range r.Keywords {
				if hasWordPrefix(s, kw) {
					out = append(out, r.Name)
					break subj
				}
			}
		}
	}
	return out
}

// hasWordPrefix: kw aparece en s al comienzo de una palabra ("war" no matchea "software").
func hasWordPrefix(s, kw string) bool {
	for from := 0; ; {
		i := strings.Index(s[from:], kw)
		if i < 0 {
			return false
		}
		i += from
		if i == 0 {
			return true
		}
		if r, _ := utf8.DecodeLastRuneInString(s[:i]); !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return true
		}
		from = i + 1
	}
}
