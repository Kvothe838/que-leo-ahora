package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CacheEntry guarda lo que se averiguó de un libro. Se guardan los subjects crudos, así
// cambiar las reglas de género no requiere volver a consultar la red.
type CacheEntry struct {
	Found        bool     `json:"found"`
	Subjects     []string `json:"subjects,omitempty"`
	Pages        int      `json:"pages,omitempty"`
	MatchedTitle string   `json:"matchedTitle,omitempty"`
	MatchedBy    string   `json:"matchedBy,omitempty"`
	GRGenres     []string `json:"grGenres,omitempty"` // géneros de la página pública de Goodreads
	GRChecked    bool     `json:"grChecked,omitempty"`
	FetchedAt    string   `json:"fetchedAt"`
}

type Cache map[string]CacheEntry

func loadCache(path string) (Cache, error) {
	c := Cache{}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s corrupto: %w", path, err)
	}
	return c, nil
}

func (c Cache) save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ") // claves ordenadas: diffs limpios en git
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type olDoc struct {
	Title      string   `json:"title"`
	AuthorName []string `json:"author_name"`
	Subject    []string `json:"subject"`
	Pages      int      `json:"number_of_pages_median"`
}

type Enricher struct {
	client *http.Client
	delay  time.Duration

	grMu   sync.Mutex // serializa y espacia las consultas a Goodreads
	grLast time.Time
}

func newEnricher() *Enricher {
	return &Enricher{client: &http.Client{Timeout: 25 * time.Second}, delay: 400 * time.Millisecond}
}

func (e *Enricher) search(ctx context.Context, q url.Values) ([]olDoc, error) {
	q.Set("fields", "title,author_name,subject,number_of_pages_median")
	q.Set("limit", "5")
	u := "https://openlibrary.org/search.json?" + q.Encode()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(e.delay * time.Duration(1<<attempt)):
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "books-helper/1.0 (personal reading tool)")
		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var body struct {
			Docs []olDoc `json:"docs"`
		}
		if resp.StatusCode == http.StatusOK {
			err = json.NewDecoder(resp.Body).Decode(&body)
		} else {
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return body.Docs, nil
	}
	return nil, lastErr
}

// Lookup busca un libro: primero por ISBN, luego por título+apellido, luego búsqueda libre.
// Devuelve error solo ante fallas de red (para no cachear un "no encontrado" falso).
func (e *Enricher) Lookup(ctx context.Context, r Row) (CacheEntry, error) {
	entry := CacheEntry{FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	title := cleanTitle(r.Title)

	type attempt struct {
		name    string
		q       url.Values
		checkTi bool
	}
	var attempts []attempt
	if r.ISBN13 != "" {
		attempts = append(attempts, attempt{"isbn", url.Values{"q": {"isbn:" + r.ISBN13}}, false})
	}
	attempts = append(attempts,
		attempt{"title+author", url.Values{"title": {title}, "author": {lastName(r.Author)}}, true},
		attempt{"free-text", url.Values{"q": {title + " " + r.Author}}, true},
	)

	var fallback *olDoc
	var fallbackBy string
	for _, a := range attempts {
		docs, err := e.search(ctx, a.q)
		if err != nil {
			return entry, err
		}
		for i := range docs {
			d := &docs[i]
			if a.checkTi && !titlesMatch(d.Title, title) {
				continue
			}
			if len(d.Subject) > 0 {
				return fill(entry, d, a.name), nil
			}
			if fallback == nil {
				fallback, fallbackBy = d, a.name
			}
		}
	}
	if fallback != nil {
		return fill(entry, fallback, fallbackBy), nil
	}
	return entry, nil
}

var subjectNoise = []string{"nyt:", "reading level", "large type", "new york times", "accessible book", "protected daisy", "in library", "lending library", "internet archive", "overdrive", "open library"}

func fill(e CacheEntry, d *olDoc, by string) CacheEntry {
	e.Found, e.Pages, e.MatchedTitle, e.MatchedBy = true, d.Pages, d.Title, by
	for _, s := range d.Subject {
		ls := strings.ToLower(s)
		noisy := false
		for _, n := range subjectNoise {
			if strings.Contains(ls, n) {
				noisy = true
				break
			}
		}
		if !noisy && len(e.Subjects) < 80 {
			e.Subjects = append(e.Subjects, s)
		}
	}
	return e
}

// cleanTitle quita paréntesis y subtítulos: "Carrie (Pocket)" -> "Carrie".
func cleanTitle(t string) string {
	if i := strings.Index(t, "("); i > 0 {
		t = t[:i]
	}
	if i := strings.Index(t, ":"); i > 0 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}

func lastName(author string) string {
	f := strings.Fields(author)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

var foldMap = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n", "à", "a", "è", "e", "ç", "c")

func fold(s string) string {
	s = foldMap.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func titlesMatch(a, b string) bool {
	fa, fb := fold(a), fold(b)
	return fa != "" && fb != "" && (strings.Contains(fa, fb) || strings.Contains(fb, fa))
}

const goodreadsInterval = 1100 * time.Millisecond

var grGenreRe = regexp.MustCompile(`genreButton"><a href="https://www\.goodreads\.com/genres/([a-z0-9-]+)"`)

// Goodreads lee los géneros de la página pública del libro (no requiere sesión).
// Máximo una consulta por segundo en total, sin importar cuántos workers haya.
func (e *Enricher) Goodreads(ctx context.Context, bookID string) ([]string, error) {
	e.grMu.Lock()
	wait := time.Until(e.grLast.Add(goodreadsInterval))
	if wait > 0 {
		select {
		case <-ctx.Done():
			e.grMu.Unlock()
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	e.grLast = time.Now()
	e.grMu.Unlock()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.goodreads.com/book/show/"+url.PathEscape(bookID), nil)
	req.Header.Set("User-Agent", "books-helper/1.0 (personal reading tool)")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("goodreads HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range grGenreRe.FindAllStringSubmatch(string(body), -1) {
		if g := strings.ReplaceAll(m[1], "-", " "); !seen[g] && len(out) < 10 {
			seen[g] = true
			out = append(out, g)
		}
	}
	return out, nil
}
