package main

import (
	"context"

	"github.com/markusylisiurunen/ship/internal/cmd"
)

func main() {
	cmd.Execute(context.Background())
}
