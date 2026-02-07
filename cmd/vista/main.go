package main

import (
	"flag"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/server"
	"log"
)

func main() {
	repoPath := flag.String("repo", ".", "Path to git repository")
	flag.Parse()

	repo, err := gitcore.NewRepository(*repoPath)
	if err != nil {
		log.Fatal(err)
	}

	serv := server.NewServer(repo, "8080")
	serv.Start()
}
