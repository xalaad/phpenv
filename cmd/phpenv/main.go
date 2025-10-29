package main

import (
	"fmt"
	"os"

	"phpenv/internal/app"
)

func main() {
	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "phpenv: %v\n", err)
		os.Exit(1)
	}
	if err := application.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "phpenv: %v\n", err)
		os.Exit(1)
	}
}
