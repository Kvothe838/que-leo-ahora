package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

func main() {
	dir := flag.String("dir", ".", "carpeta del proyecto (con data/ y exports/)")
	csvFlag := flag.String("csv", "", "export de Goodreads (por defecto, el .csv más reciente en exports/ o en la carpeta del proyecto)")
	out := flag.String("out", "", "HTML de salida (por defecto output/index.html)")
	offline := flag.Bool("offline", false, "no consultar la red: usar solo el cache")
	retry := flag.Bool("retry-missing", false, "reintentar libros que antes no se encontraron")
	noOpen := flag.Bool("no-open", false, "no abrir el navegador")
	flag.Parse()

	if err := run(*dir, *csvFlag, *out, *offline, *retry, *noOpen); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dir, csvPath, outPath string, offline, retry, noOpen bool) error {
	if csvPath == "" {
		p, err := newestCSV(dir)
		if err != nil {
			return err
		}
		csvPath = p
	}
	if outPath == "" {
		outPath = filepath.Join(dir, "output", "index.html")
	}
	cachePath := filepath.Join(dir, "data", "cache.json")

	rows, err := loadRows(csvPath)
	if err != nil {
		return err
	}
	fmt.Printf("Export: %s (%d libros)\n", csvPath, len(rows))

	cache, err := loadCache(cachePath)
	if err != nil {
		return err
	}
	rules, err := loadRules(filepath.Join(dir, "data", "genre_rules.json"))
	if err != nil {
		return fmt.Errorf("reglas de género: %w", err)
	}
	overrides, err := loadOverrides(filepath.Join(dir, "data", "overrides.json"))
	if err != nil {
		return err
	}

	if !offline {
		enrichMissing(rows, cache, cachePath, retry)
	}

	if err := renderHTML(buildPayload(rows, cache, rules, overrides, csvPath), outPath); err != nil {
		return err
	}
	abs, _ := filepath.Abs(outPath)
	fmt.Println("HTML listo:", abs)
	if noOpen {
		return nil
	}
	return openBrowser(abs)
}

// job es el trabajo pendiente de un libro: buscar en Open Library (needOL) y/o leer los
// géneros de Goodreads (fuente principal de género; Open Library queda de respaldo).
type job struct {
	row    Row
	prev   CacheEntry
	needOL bool
}

func (j job) needsGoodreads(e CacheEntry) bool {
	return !e.GRChecked
}

// enrichMissing consulta solo lo que falta en el cache.
func enrichMissing(rows []Row, cache Cache, cachePath string, retry bool) {
	var todo []job
	for _, r := range rows {
		c, ok := cache[r.ID]
		switch {
		case !ok || (retry && !c.Found):
			todo = append(todo, job{row: r, prev: c, needOL: true})
		case !c.GRChecked:
			todo = append(todo, job{row: r, prev: c})
		}
	}
	if len(todo) == 0 {
		fmt.Println("Cache al día: no hay libros nuevos que consultar.")
		return
	}
	// Primero lo que sirve para decidir qué leer.
	prio := map[string]int{"to-read": 0, "currently-reading": 1, "read": 2, "did-not-finish": 3}
	sort.SliceStable(todo, func(i, j int) bool { return prio[todo[i].row.Shelf] < prio[todo[j].row.Shelf] })

	fmt.Printf("Completando datos de %d libros (Open Library + Goodreads). Ctrl-C para pausar; se retoma después...\n", len(todo))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	e := newEnricher()

	type result struct {
		row   Row
		entry CacheEntry
		err   error
	}
	const workers = 3
	jobs := make(chan job)
	results := make(chan result)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				entry, err := j.prev, error(nil)
				if j.needOL {
					entry, err = e.Lookup(ctx, j.row)
				}
				if err == nil && j.needsGoodreads(entry) {
					var gr []string
					if gr, err = e.Goodreads(ctx, j.row.ID); err == nil {
						entry.GRGenres, entry.GRChecked = gr, true
						entry.Found = entry.Found || len(gr) > 0
					}
				}
				results <- result{j.row, entry, err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, j := range todo {
			select {
			case jobs <- j:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()

	withGenres, without, failures, done := 0, 0, 0, 0
	for res := range results {
		if ctx.Err() != nil {
			continue // descartar lo que quedó a medias al interrumpir
		}
		done++
		if res.err != nil {
			failures++
			fmt.Printf("  [%d/%d] %s: error (%v)\n", done, len(todo), res.row.Title, res.err)
			if failures >= 8 {
				fmt.Println("Demasiados errores seguidos: se corta. Reintentá más tarde.")
				stop()
			}
			continue
		}
		failures = 0
		cache[res.row.ID] = res.entry
		mark := "ok"
		if len(res.entry.Subjects)+len(res.entry.GRGenres) > 0 {
			withGenres++
		} else {
			without++
			mark = "sin géneros"
		}
		fmt.Printf("  [%d/%d] %s — %s\n", done, len(todo), res.row.Title, mark)
		if done%10 == 0 {
			_ = cache.save(cachePath)
		}
	}
	if ctx.Err() != nil {
		fmt.Println("Interrumpido; se guarda lo avanzado.")
	}
	if err := cache.save(cachePath); err != nil {
		fmt.Fprintln(os.Stderr, "no pude guardar el cache:", err)
	}
	fmt.Printf("Listo: %d con géneros, %d sin géneros.\n", withGenres, without)
}

func newestCSV(dir string) (string, error) {
	var cands []string
	for _, pat := range []string{"exports/*.csv", "*.csv"} {
		m, _ := filepath.Glob(filepath.Join(dir, pat))
		cands = append(cands, m...)
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("no encontré ningún .csv: copiá el export de Goodreads en %s", filepath.Join(dir, "exports"))
	}
	var best string
	var bestT time.Time
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && (best == "" || st.ModTime().After(bestT)) {
			best, bestT = c, st.ModTime()
		}
	}
	return best, nil
}

func openBrowser(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
