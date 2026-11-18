.PHONY: run build test offline
run:      ## actualiza datos nuevos y abre el HTML
	go run .
offline:  ## regenera el HTML solo con el cache (sin red)
	go run . -offline
build:    ## compila el binario ./books
	go build -o books .
test:
	go test ./...
