package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/divyam234/teldrive/cmd"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := cmd.New().ExecuteContext(ctx); err != nil {
		panic(err)
	}
}
