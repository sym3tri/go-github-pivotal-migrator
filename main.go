package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/coreos/pkg/flagutil"
	"github.com/google/go-github/github"
	"github.com/sym3tri/go-pivotaltracker/v5/pivotal"
	"golang.org/x/oauth2"
)

var (
	flags = struct {
		owner    string
		repos    flagutil.StringSliceFlag
		ghToken  string
		ptToken  string
		ptProjId int
		limit    int
		dryRun   bool
	}{}

	ghSvc *github.IssuesService
	ptSvc *pivotal.StoryService
)

func init() {
	flag.StringVar(&flags.owner, "owner", "", "owner of the github repositories to be checked")
	flag.Var(&flags.repos, "repos", "github repositories to be checked; if empty, all repositories of the given owner will be checked")
	flag.StringVar(&flags.ghToken, "gh-token", "", "the GitHub API access token")
	flag.StringVar(&flags.ptToken, "pt-token", "", "the Pivotal API access token")
	flag.IntVar(&flags.ptProjId, "pt-proj-id", 0, "the Pivotal Project ID")
	flag.IntVar(&flags.limit, "limit", 1000, "the max number of issues to attempt")
	flag.BoolVar(&flags.dryRun, "dry-run", true, "print actions that would be taken")
}

func main() {
	flag.Parse()
	initClients()

	repos := []string(flags.repos)
	if len(repos) < 1 {
		log.Fatal("no github repos specified")
	}

	for _, repo := range repos {
		fmt.Printf("Analysing repo: %s/%s\n", flags.owner, repo)
		iss, _, err := ghSvc.ListByRepo(flags.owner, repo, &github.IssueListByRepoOptions{
			State: "open",
			ListOptions: github.ListOptions{
				PerPage: flags.limit,
			},
		})
		if err != nil {
			log.Fatalf("failed to list issues for repo: %s, error: %v", repo, err)
		}
		fmt.Printf("found %d issues to migrate", len(iss))
		for _, is := range iss {
			migrateIssue(repo, is)
		}
	}

	fmt.Println("Finished.")
	os.Exit(0)
}

func initClients() {
	ptClient := pivotal.NewClient(flags.ptToken)
	ptSvc = ptClient.Stories

	ghClient := github.NewClient(
		func(ghToken string) *http.Client {
			if ghToken == "" {
				return nil
			}
			return oauth2.NewClient(
				oauth2.NoContext,
				oauth2.StaticTokenSource(
					&oauth2.Token{
						AccessToken: ghToken,
					},
				),
			)
		}(flags.ghToken),
	)
	ghSvc = ghClient.Issues
}

func migrateIssue(repo string, is *github.Issue) {
	fmt.Println("\n===== begin =====\n")

	var newStory *pivotal.Story
	var err error

	storyReq := convertIssue(repo, is)
	if flags.dryRun {
		printIssue(is)
		printStory(storyReq)
	} else {
		newStory, _, err = ptSvc.Create(flags.ptProjId, storyReq)
		if err != nil {
			log.Fatalf("error creating story: %v", err)
		}
	}

	ghComments, _, err := ghSvc.ListComments(flags.owner, repo, *is.Number, &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: 1000,
		},
	})
	if err != nil {
		log.Fatalf("failed to list gh comments for repo: %s, issue %d: error: %v", repo, *is.Number, err)
	}

	fmt.Printf("found %d comments for issue number: %d", len(ghComments), *is.Number)
	for _, cm := range ghComments {
		commentReq := convertComment(cm)
		if flags.dryRun {
			printIssueComment(cm)
			printStoryComment(commentReq)
		} else {
			_, _, err := ptSvc.AddComment(flags.ptProjId, newStory.Id, commentReq)
			if err != nil {
				log.Fatalf("error creating comment: %v", err)
			}
		}
	}

	fmt.Println("\n===== end =====\n")
}

func convertIssue(repo string, is *github.Issue) *pivotal.StoryRequest {
	labels := []*pivotal.Label{
		&pivotal.Label{Name: "github-migrated"},
		&pivotal.Label{Name: fmt.Sprintf("github-repo/%s", repo)},
	}

	bodyFmt := "%s\n```"
	bodyFmt += `
Migrated from Github
Created: %s
Labels: %q
`
	bodyFmt += "```\n\n"

	body := fmt.Sprintf(bodyFmt, *is.HTMLURL, *is.CreatedAt, is.Labels)
	body += *is.Body

	sr := &pivotal.StoryRequest{
		Name:        *is.Title,
		Description: body,
		Labels:      &labels,
		Type:        pivotal.StoryTypeFeature,
		State:       pivotal.StoryStateUnscheduled,
	}

	return sr
}

func convertComment(cm *github.IssueComment) *pivotal.Comment {
	bodyFmt := "%s\n```"
	bodyFmt += `
Migrated from Github
Created: %s
Author: %s
`
	bodyFmt += "```\n\n"

	body := fmt.Sprintf(bodyFmt, *cm.HTMLURL, *cm.CreatedAt, *cm.User.Login)
	body += *cm.Body

	c := &pivotal.Comment{
		Text: body,
	}

	return c
}

func printIssue(is *github.Issue) {
	fmtStr := `
--- issue ---
Number: %d
Title: %s
URL: %s
Created: %s
Labels: %q
--- /issue ---
`
	fmt.Printf(fmtStr, *is.Number, *is.Title, *is.HTMLURL, *is.CreatedAt, is.Labels)
}

func printStory(sr *pivotal.StoryRequest) {
	labels := []string{}
	for _, s := range *sr.Labels {
		labels = append(labels, s.Name)
	}

	fmtStr := `
--- story ---
Name: %s
Description: %s
Type: %s
State: %s
Labels: %q
--- /story ---
`
	fmt.Printf(fmtStr, sr.Name, trunc(sr.Description), sr.Type, sr.State, labels)
}

func printIssueComment(cm *github.IssueComment) {
	fmtStr := `
--- issue comment ---
Author: %s
Created: %s
Body: %s
--- /issue comment ---
`

	fmt.Printf(fmtStr, *cm.User.Login, *cm.CreatedAt, trunc(*cm.Body))
}

func printStoryComment(cm *pivotal.Comment) {
	fmtStr := `
--- story comment ---
Text: %s
--- /story comment ---
`
	fmt.Printf(fmtStr, trunc(cm.Text))
}

func trunc(s string) string {
	if len(s) < 255 {
		return s
	}

	return s[0:255]
}
