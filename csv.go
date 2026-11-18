package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Row es una fila del export de Goodreads, ya normalizada.
type Row struct {
	ID, Title, Author, ISBN13, Shelf string
	Tags                             []string
	Rating, Pages, Year, OrigYear    int
	DateAdded, DateRead              string
	Owned                            bool
}

var exclusiveShelves = map[string]bool{"to-read": true, "read": true, "currently-reading": true, "did-not-finish": true}

func loadRows(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReader(f))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("leyendo encabezado: %w", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.TrimPrefix(strings.TrimSpace(h), "\xef\xbb\xbf")] = i
	}
	for _, need := range []string{"Book Id", "Title", "Author", "Exclusive Shelf"} {
		if _, ok := idx[need]; !ok {
			return nil, fmt.Errorf("falta la columna %q: ¿es un export de Goodreads?", need)
		}
	}
	get := func(rec []string, col string) string {
		if i, ok := idx[col]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	var rows []Row
	for {
		rec, err := r.Read()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		if get(rec, "Book Id") == "" {
			continue
		}
		var tags []string
		for _, t := range strings.Split(get(rec, "Bookshelves"), ",") {
			if t = strings.TrimSpace(t); t != "" && !exclusiveShelves[t] {
				tags = append(tags, t)
			}
		}
		rows = append(rows, Row{
			ID:        get(rec, "Book Id"),
			Title:     get(rec, "Title"),
			Author:    strings.Join(strings.Fields(get(rec, "Author")), " "),
			ISBN13:    cleanISBN(get(rec, "ISBN13")),
			Shelf:     get(rec, "Exclusive Shelf"),
			Tags:      tags,
			Rating:    atoi(get(rec, "My Rating")),
			Pages:     atoi(get(rec, "Number of Pages")),
			Year:      atoi(get(rec, "Year Published")),
			OrigYear:  atoi(get(rec, "Original Publication Year")),
			DateAdded: strings.ReplaceAll(get(rec, "Date Added"), "/", "-"),
			DateRead:  strings.ReplaceAll(get(rec, "Date Read"), "/", "-"),
			Owned:     atoi(get(rec, "Owned Copies")) > 0,
		})
	}
	return rows, nil
}

// cleanISBN quita el formato ="123" que usa Goodreads para que Excel no rompa los ceros.
func cleanISBN(s string) string {
	return strings.Trim(strings.TrimPrefix(strings.TrimSpace(s), "="), "\"")
}

func atoi(s string) int {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int(f)
}
