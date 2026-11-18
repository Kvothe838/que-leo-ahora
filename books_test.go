package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanTitleAndMatch(t *testing.T) {
	if got := cleanTitle("Carrie (Pocket): algo"); got != "Carrie" {
		t.Fatalf("cleanTitle = %q", got)
	}
	if !titlesMatch("Cómo ganar a los fachas", "como ganar a los fachas") {
		t.Fatal("debería ignorar acentos")
	}
	if titlesMatch("Dune", "Carrie") {
		t.Fatal("no debería matchear")
	}
}

func TestClassify(t *testing.T) {
	rules, err := loadRules("data/genre_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	has := func(gs []string, g string) bool {
		for _, x := range gs {
			if x == g {
				return true
			}
		}
		return false
	}
	g := rules.Classify([]string{"Historical fiction", "Spain -- History -- Civil War, 1936-1939"})
	if !has(g, "Novela histórica") || !has(g, "Historia") || !has(g, "Ficción") {
		t.Fatalf("got %v", g)
	}
	// "History and criticism" es ruido típico de crítica literaria, no debe dar "Historia".
	if g := rules.Classify([]string{"American fiction -- History and criticism"}); has(g, "Historia") {
		t.Fatalf("falso positivo: %v", g)
	}
	// "war" no debe matchear dentro de "software".
	if !hasWordPrefix("world war", "war") || hasWordPrefix("software", "war") {
		t.Fatal("hasWordPrefix")
	}
}

func TestLoadRowsAndLiked(t *testing.T) {
	csv := "\xef\xbb\xbfBook Id,Title,Author,ISBN13,My Rating,Number of Pages,Year Published,Original Publication Year,Date Read,Date Added,Bookshelves,Exclusive Shelf,Owned Copies\n" +
		"1,Carrie,Stephen  King,\"=\"\"9781416524304\"\"\",4.0,272,1999,1974,2026/11/09,2025/01/15,favorito,read,0\n" +
		"2,It,Stephen King,\"=\"\"\"\"\",0,,1986,,,2024/01/01,\"to-read, recomendado\",to-read,1\n"
	p := filepath.Join(t.TempDir(), "e.csv")
	if err := os.WriteFile(p, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := loadRows(p)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].ISBN13 != "9781416524304" || rows[0].Author != "Stephen King" || rows[0].OrigYear != 1974 {
		t.Fatalf("%+v", rows[0])
	}
	if len(rows[1].Tags) != 1 || rows[1].Tags[0] != "recomendado" || !rows[1].Owned {
		t.Fatalf("%+v", rows[1])
	}
	pl := buildPayload(rows, Cache{"2": {Found: true, Pages: 1138}}, Rules{}, map[string]Override{}, p)
	var it OutBook
	for _, b := range pl.Books {
		if b.ID == "2" {
			it = b
		}
	}
	if it.Pages != 1138 || len(it.Liked) != 1 || it.Genres[0] != unclassified {
		t.Fatalf("%+v", it)
	}
}

func TestGenreSourcesPrefersGoodreads(t *testing.T) {
	c := CacheEntry{Subjects: []string{"History"}, GRGenres: []string{"fantasy"}}
	if got := genreSources(c); len(got) != 1 || got[0] != "fantasy" {
		t.Fatalf("got %v", got)
	}
	if got := genreSources(CacheEntry{Subjects: []string{"History"}}); got[0] != "History" {
		t.Fatalf("sin Goodreads debería usar Open Library: %v", got)
	}
}
