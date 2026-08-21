package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"cypress-servers/internal/cfb27mitm"
)

func main() {
	cfg := cfb27mitm.Config{}
	var ports string
	flag.StringVar(&cfg.Bind, "bind", "127.0.0.1", "address the capture listeners bind to")
	flag.StringVar(&ports, "ports", "27920,443,11000,44325", "comma-separated ports to intercept")
	flag.StringVar(&cfg.LogFile, "log-file", "cfb27-mitm.jsonl", "JSONL capture path")
	flag.IntVar(&cfg.BlazePort, "blaze-port", 27920, "port whose stream is parsed as Blaze frames")
	flag.Parse()

	for _, value := range strings.Split(ports, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		port, err := strconv.Atoi(value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: invalid port %q\n", value)
			os.Exit(1)
		}
		cfg.Ports = append(cfg.Ports, port)
	}

	service, err := cfb27mitm.NewService(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer service.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := service.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
