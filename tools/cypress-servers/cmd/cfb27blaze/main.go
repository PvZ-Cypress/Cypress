package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"cypress-servers/internal/cfb27blaze"
)

func main() {
	cfg := cfb27blaze.Config{}
	flag.StringVar(&cfg.Bind, "bind", "127.0.0.1", "Blaze TCP bind address")
	flag.IntVar(&cfg.Port, "port", 27920, "Blaze TCP port")
	flag.StringVar(&cfg.DiagnosticsBind, "diagnostics-bind", "127.0.0.1", "diagnostics HTTP bind address")
	flag.IntVar(&cfg.DiagnosticsPort, "diagnostics-port", 27921, "diagnostics HTTP port")
	flag.StringVar(&cfg.LogFile, "log-file", "cfb27-blaze.jsonl", "JSONL event log path")
	flag.StringVar(&cfg.RunID, "run-id", "", "shared Cypress run identifier")
	flag.StringVar(&cfg.Profile, "profile", "LocalPlayer", "local offline player name")
	flag.StringVar(&cfg.DynastyURL, "dynasty-url", "http://127.0.0.1:27910", "Cypress Dynasty REST base URL")
	flag.StringVar(&cfg.CoachCatalog, "coach-catalog", "", "authoritative CFB27 coach catalog JSON path")
	flag.BoolVar(&cfg.PersistDynasty, "persist-dynasty", true, "persist Dynasty advance transitions through the local Dynasty service")
	flag.StringVar(&cfg.TLSMode, "tls-mode", "tls13", "TLS compatibility mode: tls13, tls13-noalpn, tls12, tls12-noalpn")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := cfb27blaze.NewService(cfg).Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
