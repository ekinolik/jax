package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	snap, err := run(ctx, opts, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var out []byte
	if opts.summary {
		out, err = json.MarshalIndent(pkgconfluence.SummaryFromSnapshot(*snap), "", "  ")
	} else {
		out, err = json.MarshalIndent(snap, "", "  ")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: encode snapshot: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
