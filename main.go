package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type Issue struct {
	Id     githubv4.ID
	Number githubv4.Int
	Title  githubv4.String
	Body   githubv4.String
	State  githubv4.IssueState
	Url    githubv4.String
}

func readGithubAccount(tomlPath string, packagename string) interface{} {
	// Open and parse the TOML file
	file, err := os.Open(tomlPath)
	if err != nil {
		log.Fatalf("Failed to open TOML file: %v", err)
	}
	defer file.Close()

	tree, err := toml.LoadReader(file)
	if err != nil {
		log.Fatalf("Failed to parse TOML file: %v", err)
	}

	if tree.Has(packagename) {
		account := tree.Get(packagename + ".github_account")
		return account
	} else {
		log.Fatalf("Package %s not found in overlay.toml\n", packagename)
		return nil
	}
}

// loadRevdeps parses the deps-table relation.md and returns, for each package,
// the packages that depend on it. relation.md is an indented dependency tree of
// cat/pkg atoms: a row nested under another means the outer package depends on
// the inner one, so each parent->child edge is recorded as a reverse dep.
func loadRevdeps(path string) map[string][]string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("relation file %s unreadable (%v); skipping dependency hint", path, err)
		return nil
	}
	// pkgOf returns the cat/pkg atom on a table row and its indentation depth
	// (leading spaces before the atom); "" if the row is not a package row.
	pkgOf := func(line string) (string, int) {
		i := strings.Index(line, "|")
		if i < 0 {
			return "", 0
		}
		raw := line[:i]
		s := strings.TrimSpace(raw)
		if !strings.Contains(s, "/") || strings.ContainsAny(s, " \t") {
			return "", 0
		}
		return s, len(raw) - len(strings.TrimLeft(raw, " "))
	}
	type frame struct {
		pkg    string
		indent int
	}
	rev := map[string]map[string]bool{}
	// The table is an indented dependency tree: a row's parent (the package that
	// depends on it) is the nearest preceding row with a smaller indent. Track a
	// stack of open ancestors and record parent -> child as child's reverse dep.
	var stack []frame
	for _, line := range strings.Split(string(data), "\n") {
		pkg, indent := pkgOf(line)
		if pkg == "" {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && stack[len(stack)-1].pkg != pkg {
			parent := stack[len(stack)-1].pkg
			if rev[pkg] == nil {
				rev[pkg] = map[string]bool{}
			}
			rev[pkg][parent] = true
		}
		stack = append(stack, frame{pkg, indent})
	}
	out := map[string][]string{}
	for d, m := range rev {
		for t := range m {
			out[d] = append(out[d], t)
		}
		sort.Strings(out[d])
	}
	return out
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var (
		name         string
		newver       string
		oldver       string
		tomlFile     string
		relationFile string
	)

	flag.StringVar(&name, "name", "", "包名")
	flag.StringVar(&newver, "newver", "", "新版本号")
	flag.StringVar(&oldver, "oldver", "", "旧版本号")
	flag.StringVar(&tomlFile, "file", "", "旧版本号")
	flag.StringVar(&relationFile, "relation", "", "path to deps-table relation.md for reverse-dependency hints")
	flag.Parse()

	body := ""
	if oldver != "" {
		body += "oldver: " + oldver
	}

	gentooZhOfficialRepoName := "gentoo-zh/overlay"
	repoName := os.Getenv("GITHUB_REPOSITORY")

	if len(repoName) == 0 {
		log.Fatal("GITHUB_REPOSITORY environment is not set")
	}

	repoIsGentooZhOfficial := repoName == gentooZhOfficialRepoName

	// Reverse-dependency hint: bumping a depended-on package may require its
	// consumers to be rebuilt, so surface them in the issue instead of only the
	// version. Data comes from the deps-table relation.md (see --relation).
	if relationFile != "" {
		if deps := loadRevdeps(relationFile)[name]; len(deps) > 0 {
			body += "\n\nReverse dependencies (a bump may need these rebuilt):\n"
			const maxList = 20
			for i, d := range deps {
				if i == maxList {
					body += fmt.Sprintf("- ... and %d more\n", len(deps)-maxList)
					break
				}
				body += "- " + d + "\n"
			}
		}
	}

	// Append @github_account to body
	// Only mention user on official gentoo-zh repo
	// 根据包名读取 overlay.toml 中对应的 github_account
	account := readGithubAccount(tomlFile, name)
	if account != nil {
		switch v := account.(type) {
		case []interface{}:
			body += "\nCC:"
			for _, acc := range v {
				if repoIsGentooZhOfficial {
					body += " @" + acc.(string)
				} else {
					body += " " + acc.(string)
				}
			}
		case string:
			if repoIsGentooZhOfficial {
				body += "\nCC: @" + v
			} else {
				body += "\nCC: " + v
			}
		}
	}

	// init client
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is not set")
	}

	httpClient := oauth2.NewClient(
		context.Background(),
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		),
	)
	client := githubv4.NewClient(httpClient)

	// get nvchecker label id
	labelName := "nvchecker"
	nvcheckerLabelId := getLabelIdbyname(client, repoName, githubv4.String(labelName))

	// search existing issues
	titlePrefix := "[nvchecker] " + name + " can be bump to "
	title := titlePrefix + newver

	query := fmt.Sprintf("repo:%s is:issue in:title %s", repoName, titlePrefix)
	emptyIssue := Issue{}
	currentIssue := searchIssueByTitle(client, githubv4.String(query))

	if currentIssue != emptyIssue {
		if currentIssue.Body == githubv4.String(body) && currentIssue.Title == githubv4.String(title) {
			// If body and title match, do nothing
			return
		} else {
			// If body or title do not match
			if currentIssue.State == githubv4.IssueStateOpen {
				// If the issue is open, update it
				updateIssue(client, currentIssue, title, body, nvcheckerLabelId)
				return
			} else {
				// If the issue is closed, create a new one
				createissue(client, repoName, title, body, nvcheckerLabelId)
			}
		}
	} else {
		// If no matching issue is found, create a new one
		createissue(client, repoName, title, body, nvcheckerLabelId)
	}
}
func getRepositoryID(client *githubv4.Client, repoName string) githubv4.String {
	var q struct {
		Repository struct {
			Id githubv4.String
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(
			strings.Split(repoName, "/")[0],
		),
		"name": githubv4.String(
			strings.Split(repoName, "/")[1],
		),
	}
	err := client.Query(context.Background(), &q, variables)
	if err != nil {
		log.Fatal(err)
		return ""
	}
	return q.Repository.Id
}

func getLabelIdbyname(client *githubv4.Client, repoName string, labelName githubv4.String) githubv4.String {
	var q struct {
		Repository struct {
			Labels struct {
				Nodes []struct {
					Id   githubv4.String
					Name githubv4.String
				}
			} `graphql:"labels(first: 10, query: $labelName)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(
			strings.Split(repoName, "/")[0],
		),
		"name": githubv4.String(
			strings.Split(repoName, "/")[1],
		),
		"labelName": labelName,
	}

	err := client.Query(context.Background(), &q, variables)
	if err != nil {
		log.Fatalf("Failed to fetch labels: %v", err)
	}

	if len(q.Repository.Labels.Nodes) == 0 {
		log.Printf("label %s not found in repository %s, creating ...", labelName, repoName)
		var m struct {
			CreateLabel struct {
				Label struct {
					Id githubv4.String
				}
			} `graphql:"createLabel(input: $input)"`
		}
		input := githubv4.CreateLabelInput{
			RepositoryID: getRepositoryID(client, repoName),
			Name:         labelName,
			Color:        githubv4.String("5319e7"), // purple
			Description:  githubv4.NewString(githubv4.String("Labels created by bumpbot using nvchecker")),
		}
		err := client.Mutate(context.Background(), &m, input, nil)
		if err != nil {
			log.Fatalf("Failed to create label: %v", err)
		}
		return m.CreateLabel.Label.Id
	}

	for _, node := range q.Repository.Labels.Nodes {
		if node.Name == labelName {
			return node.Id
		}
	}
	return ""
}

func searchIssueByTitle(client *githubv4.Client, query githubv4.String) Issue {

	emptyIssue := Issue{}
	var q struct {
		Search struct {
			Nodes []struct {
				Issue `graphql:"... on Issue"`
			}
		} `graphql:"search(query: $query, type: ISSUE, first: 1)"`
	}

	err := client.Query(
		context.Background(),
		&q,
		map[string]interface{}{"query": query},
	)

	if err != nil {
		log.Fatalf("Failed to search issue: %v", err)
		return emptyIssue
	}

	if len(q.Search.Nodes) == 1 {
		for _, node := range q.Search.Nodes {
			return node.Issue
		}
	}
	return emptyIssue
}

func createissue(client *githubv4.Client, repoName string, title string, body string, labelId githubv4.ID) {
	var m struct {
		CreateIssue struct {
			Issue struct {
				Url githubv4.String
			}
		} `graphql:"createIssue(input: $input)"`
	}

	input := githubv4.CreateIssueInput{
		RepositoryID: getRepositoryID(client, repoName),
		Title:        githubv4.String(title),
		Body:         githubv4.NewString(githubv4.String(body)),
		LabelIDs:     &[]githubv4.ID{labelId},
	}

	err := client.Mutate(context.Background(), &m, input, nil)
	if err != nil {
		log.Fatalf("Failed to create issue: %v", err)
	}

	fmt.Printf("Created issue: %s\n", m.CreateIssue.Issue.Url)
}

func updateIssue(client *githubv4.Client, issue Issue, title string, body string, labelId githubv4.ID) {
	var m struct {
		UpdateIssue struct {
			Issue struct {
				Url githubv4.String
			}
		} `graphql:"updateIssue(input: $input)"`
	}

	input := githubv4.UpdateIssueInput{
		ID:       issue.Id,
		Title:    githubv4.NewString(githubv4.String(title)),
		Body:     githubv4.NewString(githubv4.String(body)),
		LabelIDs: &[]githubv4.ID{labelId},
	}

	err := client.Mutate(context.Background(), &m, input, nil)
	if err != nil {
		log.Fatalf("Failed to update issue: %v", err)
	}

	fmt.Printf("Updated issue: %s\n", m.UpdateIssue.Issue.Url)
}
