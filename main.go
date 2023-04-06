package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var (
	githubToken, openAiToken string
	usage                    int
)

func main() {
	defer fmt.Printf("Used %d tokens\n", usage)

	repoOwner := os.Getenv("GITHUB_REPOSITORY")
	repo := os.Getenv("GITHUB_REPOSITORY_OWNER")

	fmt.Printf("repo: %s repoOwner: %s\n", repo, repoOwner)

	err := env()
	if err != nil {
		fmt.Printf("%s / %s / %s", githubToken, openAiToken, err)
		os.Exit(1)
	}

	joke, err := Prompt("Tell me a Joke")
	if err != nil {
		fmt.Printf("unable to chat: %s", err)
		os.Exit(2)
	}

	fmt.Printf("%#v", joke)
}

func postComment(joke string, repoOwner string, repo string, prNumber int) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	comment := &github.IssueComment{
		Body: github.String(joke),
	}

	comment, _, err := client.Issues.CreateComment(ctx, repoOwner, repo, prNumber, comment)
	if err != nil {
		fmt.Printf("error posting comment: %s\n", err)
		os.Exit(1)
	}
}

func env() error {
	githubToken = os.Getenv("GITHUB_TOKEN")
	openAiToken = os.Getenv("OPENAI_TOKEN")

	if githubToken == "" || openAiToken == "" {
		return fmt.Errorf("you need to set both GITHUB_TOKEN and OPENAI_TOKEN")
	}

	return nil
}
