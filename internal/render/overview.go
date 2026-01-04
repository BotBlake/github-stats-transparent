package render

import (
	"os"
	"text/template"

	"github.com/botblake/github-stats-transparent/internal/stats"
)

func GenerateOverview(s *stats.Stats) error {
	tpl, err := template.ParseFiles("templates/overview.svg")
	if err != nil {
		return err
	}

	os.MkdirAll("generated", 0755)
	f, err := os.Create("generated/overview.svg")
	if err != nil {
		return err
	}
	defer f.Close()

	return tpl.Execute(f, s)
}
