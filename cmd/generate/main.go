package main

import (
	"context"
	"log"
	"os"

	"github.com/botblake/github-stats-transparent/internal/github"
	"github.com/botblake/github-stats-transparent/internal/render"
	"github.com/botblake/github-stats-transparent/internal/stats"
)

func main() {
	token := os.Getenv("ACCESS_TOKEN")
	user := os.Getenv("GITHUB_ACTOR")

	if token == "" || user == "" {
		log.Fatal("ACCESS_TOKEN and GITHUB_ACTOR must be set")
	}

	client := github.NewClient(user, token, 10)
	s := stats.NewStats(user, client)

	ctx := context.Background()
	if err := s.Collect(ctx); err != nil {
		log.Fatal(err)
	}

	if err := render.GenerateOverview(s); err != nil {
		log.Fatal(err)
	}
	if err := render.GenerateLanguages(s); err != nil {
		log.Fatal(err)
	}
}
