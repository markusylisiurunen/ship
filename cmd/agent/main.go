package main

import (
	"context"

	"github.com/markusylisiurunen/ship/internal/agent"
)

var version = "dev"

func main() {
	agent.Execute(context.Background(), version)
}
