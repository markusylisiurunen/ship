package main

import (
	"context"
	"ship/internal/cmd"
)

func main() {
	cmd.Execute(context.Background())
}
