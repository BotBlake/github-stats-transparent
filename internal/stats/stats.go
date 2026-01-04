package stats

import (
	"context"
	"fmt"

	"sync"

	"github.com/botblake/github-stats-transparent/internal/github"
)

type LanguageStat struct {
	Size        int
	Occurrences int
	Color       string
	Prop        float64
}

type Stats struct {
	Username string
	Client   *github.Client

	ExcludeRepos map[string]struct{}
	ExcludeLangs map[string]struct{}
	CountForks   bool

	Name         string
	Stargazers   int
	Forks        int
	TotalContrib int

	Languages map[string]*LanguageStat
	Repos     map[string]struct{}
}

func NewStats(username string, client *github.Client) *Stats {
	return &Stats{
		Username:  username,
		Client:    client,
		Languages: make(map[string]*LanguageStat),
		Repos:     make(map[string]struct{}),
	}
}

func (s *Stats) Collect(ctx context.Context) error {
	query := `
query {
  viewer {
    login
    name
    repositories(first: 100, isFork: false) {
      nodes {
        nameWithOwner
        stargazers { totalCount }
        forkCount
        languages(first: 10, orderBy: {field: SIZE, direction: DESC}) {
          edges {
            size
            node { name color }
          }
        }
      }
    }
  }
}`

	resp, err := s.Client.QueryGraphQL(ctx, query)
	if err != nil {
		return err
	}

	viewer := resp["data"].(map[string]any)["viewer"].(map[string]any)

	s.Name = viewer["name"].(string)

	repos := viewer["repositories"].(map[string]any)["nodes"].([]any)

	for _, r := range repos {
		repo := r.(map[string]any)
		name := repo["nameWithOwner"].(string)
		s.Repos[name] = struct{}{}

		s.Stargazers += int(repo["stargazers"].(map[string]any)["totalCount"].(float64))
		s.Forks += int(repo["forkCount"].(float64))

		langs := repo["languages"].(map[string]any)["edges"].([]any)
		for _, l := range langs {
			lang := l.(map[string]any)
			node := lang["node"].(map[string]any)
			langName := node["name"].(string)

			size := int(lang["size"].(float64))
			color := ""
			if node["color"] != nil {
				color = node["color"].(string)
			}

			stat := s.Languages[langName]
			if stat == nil {
				stat = &LanguageStat{Color: color}
				s.Languages[langName] = stat
			}
			stat.Size += size
			stat.Occurrences++
		}
	}

	total := 0
	for _, v := range s.Languages {
		total += v.Size
	}
	for _, v := range s.Languages {
		v.Prop = 100 * float64(v.Size) / float64(total)
	}

	return nil
}

func (s *Stats) LinesChanged(ctx context.Context) (int, int, error) {
	var add, del int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for repo := range s.Repos {
		wg.Add(1)
		go func() {
			defer wg.Done()

			resp, err := s.Client.QueryREST(ctx, "/repos/"+repo+"/stats/contributors")
			if err != nil {
				return
			}

			for _, a := range resp {
				author, ok := a.(map[string]any)["author"].(map[string]any)
				if !ok {
					continue
				}

				if author["login"] != s.Username {
					continue
				}

				for _, w := range a.(map[string]any)["weeks"].([]any) {
					week := w.(map[string]any)

					mu.Lock()
					add += int(week["a"].(float64))
					del += int(week["d"].(float64))
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()
	return add, del, nil
}

func (s *Stats) String() string {
	return fmt.Sprintf(
		"Name: %s\nStars: %d\nForks: %d\nRepos: %d",
		s.Name,
		s.Stargazers,
		s.Forks,
		len(s.Repos),
	)
}
