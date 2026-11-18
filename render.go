package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

const likedMinRating = 4

// Override permite corregir a mano géneros o páginas (data/overrides.json, clave = Book Id).
type Override struct {
	Genres []string `json:"genres,omitempty"`
	Pages  int      `json:"pages,omitempty"`
}

type LikedRef struct {
	Title  string `json:"t"`
	Rating int    `json:"r"`
}

type OutBook struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Author    string     `json:"author"`
	Pages     int        `json:"pages"`
	Year      int        `json:"year"`
	Shelf     string     `json:"shelf"`
	Rating    int        `json:"rating"`
	DateAdded string     `json:"added"`
	DateRead  string     `json:"read,omitempty"`
	Owned     bool       `json:"owned,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Genres    []string   `json:"genres"`
	Liked     []LikedRef `json:"liked,omitempty"`
}

type Payload struct {
	GeneratedAt string              `json:"generatedAt"`
	Source      string              `json:"source"`
	LikedMin    int                 `json:"likedMin"`
	GenreOrder  []string            `json:"genreOrder"`
	Overrides   map[string]Override `json:"overrides"`
	Books       []OutBook           `json:"books"`
}

func authorKey(a string) string { return strings.ToLower(strings.Join(strings.Fields(a), " ")) }

func loadOverrides(path string) (map[string]Override, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Override{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]Override{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

func buildPayload(rows []Row, cache Cache, rules Rules, ov map[string]Override, source string) Payload {
	// Autores que me gustaron: libros leídos con rating >= likedMinRating.
	liked := map[string][]LikedRef{}
	for _, r := range rows {
		if r.Shelf == "read" && r.Rating >= likedMinRating {
			k := authorKey(r.Author)
			liked[k] = append(liked[k], LikedRef{r.Title, r.Rating})
		}
	}

	p := Payload{GeneratedAt: time.Now().Format("2006-01-02 15:04"), Source: filepath.Base(source), LikedMin: likedMinRating, Overrides: ov}
	if p.Overrides == nil {
		p.Overrides = map[string]Override{}
	}
	for _, r := range rules {
		p.GenreOrder = append(p.GenreOrder, r.Name)
	}
	p.GenreOrder = append(p.GenreOrder, unclassified)

	for _, r := range rows {
		c := cache[r.ID]
		o := ov[r.ID]

		pages := r.Pages
		if pages == 0 {
			pages = c.Pages
		}
		if o.Pages > 0 {
			pages = o.Pages
		}
		genres := o.Genres
		if len(genres) == 0 {
			genres = rules.Classify(genreSources(c))
		}
		if len(genres) == 0 {
			genres = []string{unclassified}
		}
		year := r.OrigYear
		if year == 0 {
			year = r.Year
		}
		b := OutBook{
			ID: r.ID, Title: r.Title, Author: r.Author, Pages: pages, Year: year, Shelf: r.Shelf,
			Rating: r.Rating, DateAdded: r.DateAdded, DateRead: r.DateRead, Owned: r.Owned,
			Tags: r.Tags, Genres: genres,
		}
		if r.Shelf != "read" {
			b.Liked = liked[authorKey(r.Author)]
		}
		p.Books = append(p.Books, b)
	}
	sort.SliceStable(p.Books, func(i, j int) bool { return p.Books[i].Title < p.Books[j].Title })
	return p
}

func renderHTML(p Payload, outPath string) error {
	tpl, err := webFS.ReadFile("web/index.html")
	if err != nil {
		return err
	}
	data, err := json.Marshal(p) // json.Marshal escapa <, > y & : seguro dentro de <script>
	if err != nil {
		return err
	}
	const marker = "/*__DATA__*/ {}"
	html := string(tpl)
	if !strings.Contains(html, marker) {
		return fmt.Errorf("web/index.html no contiene el marcador %q", marker)
	}
	html = strings.Replace(html, marker, string(data), 1)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(html), 0o644)
}

// maxGRGenres: Goodreads ordena sus géneros por popularidad; los últimos suelen ser ruido.
const maxGRGenres = 6

// genreSources elige de dónde clasificar: Goodreads si trae géneros, si no Open Library.
func genreSources(c CacheEntry) []string {
	if len(c.GRGenres) > 0 {
		return c.GRGenres[:min(len(c.GRGenres), maxGRGenres)]
	}
	return c.Subjects
}
