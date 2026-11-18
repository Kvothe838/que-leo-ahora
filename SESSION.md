# Sesión de construcción — books-helper (2026-11-17)

Resumen de lo que se hizo, por qué, y cómo retomar. El uso diario está en `README.md`.

## Objetivo
Filtrar el export de Goodreads (433 libros: 277 por leer) para decidir qué leer. Ejemplo guía:
"libro histórico de hasta 200 páginas". Debe vivir en local, ser fácil de ejecutar (un comando que abre el HTML)
y mantenerse en el tiempo: al cargar un export nuevo, solo se consultan los libros nuevos.

## Decisiones
| Tema | Decisión | Motivo |
|---|---|---|
| Lenguaje | Go, solo librería estándar | Binario único, `go:embed` para el HTML, sin dependencias |
| UI | HTML autocontenido (`output/index.html`) con datos embebidos | Sin servidor; se abre con doble clic |
| Género | **Goodreads** (página pública, por Book Id) como fuente principal; Open Library como respaldo | El CSV no trae género. Open Library no cubre ediciones en español (63 no encontrados y 73 sin etiquetas de 277). Google Books sin clave: cuota anónima agotada |
| Clasificación | Reglas por palabras clave en `data/genre_rules.json` (editables, multi-idioma) sobre los géneros crudos del cache | Cambiar reglas no requiere red (`make offline`) |
| Correcciones | `data/overrides.json` (clave = Book Id) | Manual y versionable |
| Chrome | No se usó para clasificar | Goodreads responde por HTTP simple sin sesión. Chrome solo se usó para probar la UI |
| Idioma como filtro | Descartado | Pedido del usuario |

## Estado al cerrar
- Pendientes sin clasificar: de 141 a 21 (31 contando todos los estantes).
- "Histórico ≤ 200 págs" devuelve 8 libros; algunos con "Novela histórica" dudosa (p. ej. *La llamada de la selva*).
- `data/overrides.json` sigue vacío (`{}`).
- Tests (`make test`) pasan. UI probada en Chrome: carga, input de páginas, vista de clasificar y exportación.
- No hay repositorio git todavía.

## Funcionalidades de la página
Filtros: estante, páginas (rango y atajos), género (alguno/todos), año original, etiquetas propias, búsqueda,
"autor que ya leí y me gustó" (≥ 4★), "lo tengo en casa". Orden múltiple, botón 🎲 Sorprendeme, y vista oculta
**🏷 clasificar (N)** en el encabezado para asignar géneros a mano y exportar un `overrides.json` fusionado.

## Problemas encontrados y resueltos
1. **Open Library con cobertura baja** → respaldo con Goodreads, que pasó a ser la fuente principal.
2. **Corrida lenta (~10 s por libro)** → 3 workers; Goodreads limitado a 1 consulta/s global.
3. **Falsos positivos de género** → `histoires` (francés = historias) excluido de Historia; `non fiction` excluido de Ficción y agregado "No ficción".
4. **Input de páginas escribía "001"** → cada tecla reconstruía el panel y movía el cursor; ahora al escribir solo se refrescan resultados y conteos.
5. **Vista de clasificar vacía** → se pasaba un arreglo a `replaceChildren`; ahora se expande.
6. **`alert()` bloqueante en "Copiar JSON"** → aviso dentro del botón (cambio sin probar en navegador).

## Cómo retomar
```sh
make run        # export nuevo en exports/ → consulta solo lo nuevo → abre el HTML
make offline    # regenera con el cache (tras editar reglas u overrides)
make test
```
Archivos clave: `main.go` (orquestación y workers), `enrich.go` (Open Library y Goodreads), `genres.go` (clasificador),
`render.go` (payload y overrides), `csv.go`, `web/index.html` (UI).

## Pendientes / ideas
- Clasificar los 31 libros restantes con la vista 🏷 y reemplazar `data/overrides.json`.
- Afinar `genre_rules.json`: "Novela histórica" es demasiado amplia.
- `git init` y subir a GitHub; `.gitignore` ya excluye `*.csv` y `output/`. Versionar `data/cache.json`.
- Opcional: clasificación con Claude API para lo que Goodreads no etiquete (requiere clave propia y envía títulos y autores).
- Opcional: modo `-serve` para que la UI escriba `overrides.json` directamente, sin descargar el archivo.
