# books-helper

Filtra tu biblioteca de Goodreads (género, páginas, año, autores que ya te gustaron…) para decidir qué leer.
Todo corre en local: un programa en Go (solo librería estándar) genera un HTML autocontenido y lo abre en el navegador.

## Uso

```sh
make run          # o: go run .
```

Lee el `.csv` más reciente en `exports/` (o en la raíz), consulta **solo los libros que no están en el cache**
y abre `output/index.html`.

### Actualizar con un export nuevo

1. En Goodreads: *My Books → Import and export → Export Library*.
2. Copiá el CSV a `exports/` (el más reciente gana; no hace falta borrar los viejos).
3. `make run`. Los libros ya conocidos no se vuelven a consultar.

### Flags

| Flag | Qué hace |
|---|---|
| `-offline` / `make offline` | No usa la red; regenera el HTML con el cache |
| `-retry-missing` | Reintenta los libros que antes no se encontraron |
| `-csv ruta.csv` | Usa un export específico |
| `-no-open` | No abre el navegador |

Ctrl-C durante la búsqueda es seguro: lo avanzado se guarda y se retoma en la próxima corrida.

## Qué se versiona (para sincronizar entre PCs)

| Archivo | Rol |
|---|---|
| `data/cache.json` | Datos obtenidos de Open Library por Book Id (subjects, páginas). **Se sube a git**, así la otra PC no repite consultas |
| `data/genre_rules.json` | Palabras clave → géneros propios. Editable; tras editar, `make offline` y listo (no usa la red) |
| `data/overrides.json` | Correcciones manuales por Book Id |
| `output/index.html` | **Se sube**: es la página que publica GitHub Pages |
| `exports/*.csv` | **No se sube** (el CSV trae tus reseñas y notas privadas) |

### Corregir un libro a mano

En `data/overrides.json`, la clave es el Book Id (está en la URL de Goodreads):

```json
{ "10644930": { "genres": ["Ficción", "Ciencia ficción"], "pages": 849 } }
```

### Clasificar los "Sin clasificar" desde la página

En el encabezado hay un botoncito discreto **🏷 clasificar (N)**. Abre una vista con los libros sin género, con links a su
página de Goodreads y a una búsqueda en Google, y chips para elegir géneros (y páginas si faltan). Se aplican al instante y
quedan recordados en el navegador. Para hacerlos permanentes: **Descargar overrides.json**, reemplazá `data/overrides.json`
(el archivo ya incluye los overrides anteriores) y corré `make offline`.

### Libros "Sin clasificar" o con género raro

Mirá `data/cache.json`: `matchedTitle` muestra con qué libro de Open Library se emparejó y `subjects` qué etiquetas trae.
Si el emparejamiento es incorrecto, corregilo con un override; si faltan palabras clave, agregalas en `genre_rules.json`.

## De dónde salen los datos

- **Goodreads CSV**: título, autor, páginas, año, estante, rating, fecha de alta, etiquetas propias.
- **Open Library** (gratis, sin clave): *subjects* (para el género) y páginas cuando el CSV no las trae.
  Solo se envían título, autor e ISBN de libros, nunca reseñas ni datos personales.
- **Goodreads (página pública del libro)**: si Open Library no trae géneros, se leen los géneros de
  `goodreads.com/book/show/<Book Id>` (sin sesión, máx. 1 consulta por segundo). Se guardan en `grGenres` del cache.
- "Autor que me gustó" = autor con algún libro **leído** con rating ≥ 4 (`likedMinRating` en `render.go`).

## Publicar en GitHub Pages

`output/index.html` se publica solo con cada push (workflow `.github/workflows/pages.yml`).

```sh
make run          # o make offline
git add -A && git commit -m "Actualizar" && git push
```

Pocos segundos después, la página muestra el nuevo `index.html`.

## Desarrollo

```sh
make test
make build        # genera ./books
```
