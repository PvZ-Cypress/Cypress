package main

import (
	"flag"
	"fmt"
	"os"

	"cypress-servers/internal/dynasty"
)

func main() {
	cfg := dynasty.Config{}
	flag.StringVar(&cfg.Bind, "bind", "0.0.0.0", "HTTP bind address")
	flag.IntVar(&cfg.Port, "port", 27910, "HTTP/WebSocket port")
	flag.StringVar(&cfg.SchemaRoot, "schema-root", `C:\Users\Shadow\Desktop\CFB27\Dynasty_Files`, "CFB27 Dynasty .FTX schema root")
	flag.StringVar(&cfg.DBFile, "db", "cfb27_dynasty.db", "SQLite database file")
	flag.StringVar(&cfg.SeedFile, "seed", "", "full FBCHUNKS Dynasty save used as an immutable session seed")
	flag.StringVar(&cfg.DataDir, "data-dir", "", "directory for per-session Dynasty save artifacts")
	flag.StringVar(&cfg.NodePath, "node", "node", "Node.js executable used by the Dynasty franchise mutator")
	flag.StringVar(&cfg.FranchiseTool, "franchise-tool", "", "path to cmd/cfb27franchise/main.mjs")
	flag.Parse()

	if err := dynasty.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
