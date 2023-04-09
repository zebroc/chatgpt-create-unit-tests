package main

import (
	"context"
	"fmt"
	"github.com/google/go-github/v51/github"
	"golang.org/x/oauth2"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	githubToken, openAiToken, workspaceDir string
	repoOwner, repoName, ref               string
	usage                                  int
	patchFileName                          = "patch"
	prompts                                = map[string]string{
		"Unit tests":  "If there are any new functions in this patch, write a unit test for each of them\n\n%s",
		"Code review": "Please perform a code review for this patch:\n\n%s",
	}
)

func main() {
	defer fmt.Printf("Used %d OpenAI tokens\n", usage)

	err := env()
	if err != nil {
		Exit(fmt.Sprintf("unable to determine OpenAI token or GitHub token: %v\n", err), 1)
	}

	patch, err := getPatch(workspaceDir + "/" + patchFileName)
	if err != nil {
		Exit(fmt.Sprintf("unable to get patch: %s\n", err), 2)
	}

	var wg sync.WaitGroup
	wg.Add(len(prompts))
	for name, prompt := range prompts {
		PromptAndComment(patch, name, prompt, &wg)
	}
	wg.Wait()
}

// PromptAndComment executes prompt with patch and creates a comment or logs an error
func PromptAndComment(patch []byte, name, prompt string, wg *sync.WaitGroup) {
	defer wg.Done()

	p := fmt.Sprintf(prompt, patch)
	response, err := Prompt(p)
	if err != nil {
		fmt.Printf("unable to prompt ChatGTP: %s\n", err)
		return
	}

	if len(response.Choices) <= 0 || response.Choices[0].Message.Content == "" {
		fmt.Printf("no or empty response from ChatGPT: %#v\n", response)
		return
	}

	fmt.Printf("Promt response for %s: %s", name, response.Choices[0].Message.Content)

	err = postComment(response.Choices[0].Message.Content, repoOwner, repoName)
	if err != nil {
		fmt.Printf("unable to post comment: %v\n", err)
	}
}

// getPatch loads the data from file f and returns it or an error
func getPatch(f string) ([]byte, error) {
	file, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	patch, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return patch, nil
}

// postComment posts c as a comment on PR prNumber or returns an error
func postComment(c, repoOwner, repo string) error {
	x := strings.Split(ref, "/")
	prNumber, err := strconv.Atoi(x[2])
	if err != nil {
		return err
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	comment := &github.IssueComment{
		Body: github.String(c),
	}

	comment, _, err = client.Issues.CreateComment(ctx, repoOwner, repo, prNumber, comment)
	if err != nil {
		return err
	}

	return nil
}

// env sets some variables from the environment and returns an error if required variables aren't set
func env() error {
	githubToken = os.Getenv("GITHUB_TOKEN")
	openAiToken = os.Getenv("OPENAI_TOKEN")

	ref = os.Getenv("GITHUB_REF")
	workspaceDir = os.Getenv("GITHUB_WORKSPACE")

	if x := strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/"); len(x) == 2 {
		repoOwner = x[0]
		repoName = x[1]
	} else {
		return fmt.Errorf("GITHUB_REPOSITORY was in wrong format: %s", os.Getenv("GITHUB_REPOSITORY"))
	}

	if githubToken == "" || openAiToken == "" {
		return fmt.Errorf("you need to set both GITHUB_TOKEN and OPENAI_TOKEN")
	}

	return nil
}

func Exit(m string, i int) {
	fmt.Print(m)
	os.Exit(i)
}
