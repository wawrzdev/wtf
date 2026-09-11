package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wawrzdev/wtf/internal/app"
)

var version = "dev"

func main() {
	if err := app.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version); err != nil {
		fmt.Fprintf(os.Stderr, "wtf: %v\n", err)
		os.Exit(1)
	}
}
