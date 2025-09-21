package main

import (
	"context"

	"github.com/markusylisiurunen/ship/internal/client"
)

var version = "dev"

func main() {
	client.Execute(context.Background(), version)
}
