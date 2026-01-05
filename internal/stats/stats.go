package stats

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/botblake/github-stats-transparent/internal/github"
)

type LanguageStat struct {
	Size        int
	Occurrences int
	Color       string
	Prop        float64
}

type LanguageRender struct {
	Name        string
	Color       string
	Percent     float64
	BarWidth    float64
	MarginRight float64
	DelayMS     int
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

	Lines int
	Views int

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

//
// ─────────────────────────────────────────────────────────────
//  MAIN COLLECTION
// ─────────────────────────────────────────────────────────────
//

func (s *Stats) Collect(ctx context.Context) error {
	if err := s.collectReposAndLanguages(ctx); err != nil {
		return err
	}

	s.computeLanguagePercentages()

	add, del, err := s.collectLinesChanged(ctx)
	if err != nil {
		return err
	}
	s.Lines = add + del

	if err := s.collectViews(ctx); err != nil {
		return err
	}

	if err := s.collectTotalContributions(ctx); err != nil {
		return err
	}

	return nil
}

//
// ─────────────────────────────────────────────────────────────
//  REPOS + LANGUAGES
// ─────────────────────────────────────────────────────────────
//

func (s *Stats) collectReposAndLanguages(ctx context.Context) error {
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

	if viewer["name"] != nil {
		s.Name = viewer["name"].(string)
	} else {
		s.Name = viewer["login"].(string)
	}

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

			color := "#000000"
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

	return nil
}

func (s *Stats) computeLanguagePercentages() {
	total := 0
	for _, v := range s.Languages {
		total += v.Size
	}
	for _, v := range s.Languages {
		v.Prop = 100 * float64(v.Size) / float64(total)
	}
}

//
// ─────────────────────────────────────────────────────────────
//  LANGUAGE PROGRESS (SVG MODEL)
// ─────────────────────────────────────────────────────────────
//

func (s *Stats) LanguageProgress() []LanguageRender {
	type kv struct {
		Name string
		Stat *LanguageStat
	}

	var list []kv
	for name, stat := range s.Languages {
		list = append(list, kv{name, stat})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Stat.Size > list[j].Stat.Size
	})

	var out []LanguageRender
	delay := 150

	for i, item := range list {
		ratioA, ratioB := 0.98, 0.02
		if item.Stat.Prop > 50 {
			ratioA, ratioB = 0.99, 0.01
		}
		if i == len(list)-1 {
			ratioA, ratioB = 1, 0
		}

		out = append(out, LanguageRender{
			Name:        item.Name,
			Color:       item.Stat.Color,
			Percent:     item.Stat.Prop,
			BarWidth:    ratioA * item.Stat.Prop,
			MarginRight: ratioB * item.Stat.Prop,
			DelayMS:     i * delay,
		})
	}

	return out
}

//
// ─────────────────────────────────────────────────────────────
//  LINES CHANGED
// ─────────────────────────────────────────────────────────────
//

func (s *Stats) collectLinesChanged(ctx context.Context) (int, int, error) {
	var add, del int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for repo := range s.Repos {
		repo := repo
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

//
// ─────────────────────────────────────────────────────────────
//  VIEWS (TRAFFIC API)
// ─────────────────────────────────────────────────────────────
//

func (s *Stats) collectViews(ctx context.Context) error {
	var wg sync.WaitGroup
	var mu sync.Mutex

	total := 0

	for repo := range s.Repos {
		repo := repo
		wg.Add(1)

		go func() {
			defer wg.Done()

			resp, err := s.Client.QueryREST(ctx, "/repos/"+repo+"/traffic/views")
			if err != nil {
				return
			}

			views, ok := resp["views"].([]any)
			if !ok {
				return
			}

			local := 0
			for _, v := range views {
				view := v.(map[string]any)
				local += int(view["count"].(float64))
			}

			mu.Lock()
			total += local
			mu.Unlock()
		}()
	}

	wg.Wait()
	s.Views = total
	return nil
}

//
// ─────────────────────────────────────────────────────────────
//  TOTAL CONTRIBUTIONS (YEARS)
// ─────────────────────────────────────────────────────────────
//

func (s *Stats) collectTotalContributions(ctx context.Context) error {
	yearsQuery := `
query {
  viewer {
    contributionsCollection {
      contributionYears
    }
  }
}`

	resp, err := s.Client.QueryGraphQL(ctx, yearsQuery)
	if err != nil {
		return err
	}

	years := resp["data"].(map[string]any)["viewer"].(map[string]any)["contributionsCollection"].(map[string]any)["contributionYears"].([]any)

	if len(years) == 0 {
		return nil
	}

	var yearFragments string
	for _, y := range years {
		year := int(y.(float64))
		yearFragments += fmt.Sprintf(`
year%d: contributionsCollection(
  from: "%d-01-01T00:00:00Z",
  to: "%d-01-01T00:00:00Z"
) {
  contributionCalendar {
    totalContributions
  }
}`, year, year, year+1)
	}

	query := fmt.Sprintf(`
query {
  viewer {
    %s
  }
}`, yearFragments)

	resp, err = s.Client.QueryGraphQL(ctx, query)
	if err != nil {
		return err
	}

	viewer := resp["data"].(map[string]any)["viewer"].(map[string]any)

	total := 0
	for _, v := range viewer {
		year := v.(map[string]any)
		total += int(
			year["contributionCalendar"].(map[string]any)["totalContributions"].(float64),
		)
	}

	s.TotalContrib = total
	return nil
}

//
// ─────────────────────────────────────────────────────────────
//  STRING
// ─────────────────────────────────────────────────────────────
//

func (s *Stats) String() string {
	return fmt.Sprintf(
		"Name: %s\nStars: %d\nForks: %d\nRepos: %d\nViews: %d\nLines Changed: %d\nTotal Contributions: %d",
		s.Name,
		s.Stargazers,
		s.Forks,
		len(s.Repos),
		s.Views,
		s.LinesChanged,
		s.TotalContrib,
	)
}
