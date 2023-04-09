package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/google/go-github/v51/github"
	"golang.org/x/oauth2"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

var (
	debug                                  bool
	githubToken, openAiToken, workspaceDir string
	repoOwner, repoName, ref, base, head   string
	usages                                 []int
	patchFileName                          = "patch"
	prompts                                = map[string]string{
		"Unit tests":         "If there are any new functions in this patch, write a unit test for each of them\n\n%s",
		"Code review":        "Please perform a code review for this patch:\n\n%s",
		"Scalability review": "Review the given patch for potential scalability issues:\n\n%s",
		"Security review":    "Review the given patch for potential security issues:\n\n%s",
	}
)

func main() {
	defer printTokenUsage(usages)

	err := env()
	if err != nil {
		_ = postComment("unable to determine OpenAI token or GitHub token", repoOwner, repoName, ref)
		Exit(fmt.Sprintf("unable to determine OpenAI token or GitHub token: %v\n", err), 1)
	}

	patch, err := getPatch()
	if err != nil {
		_ = postComment(fmt.Sprintf("unable to get patch: %s\n", err), repoOwner, repoName, ref)
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
	if wg != nil {
		defer wg.Done()
	}

	p := fmt.Sprintf(prompt, string(patch))
	DebugPrint("Pompting: %s", p)
	response, err := Prompt(p)
	if err != nil {
		msg := fmt.Sprintf("unable to prompt ChatGTP: %s\n", err)
		fmt.Print(msg)
		_ = postComment(msg, repoOwner, repoName, ref)
		return
	}

	if len(response.Choices) <= 0 || response.Choices[0].Message.Content == "" {
		msg := fmt.Sprintf("no or empty response from ChatGPT: %#v\n", response)
		fmt.Print(msg)
		_ = postComment(msg, repoOwner, repoName, ref)
		return
	}

	DebugPrint("Promt response for %s: %s", name, response.Choices[0].Message.Content)

	err = postComment(response.Choices[0].Message.Content, repoOwner, repoName, ref)
	if err != nil {
		fmt.Printf("unable to post comment: %v\n", err)
	}
}

// getPatch tries to get a patch handed in via the workspace or from the source code in the container
// running the action. It returns whatever works out or an error.
func getPatch() ([]byte, error) {
	patchFromWorkspace, errWS := getPatchFromWorkspace(workspaceDir + "/" + patchFileName)
	patchFromFS, errFS := getPatchFromFilesystem(base, head)

	switch {
	case errWS == nil && errFS == nil:
		if bytes.Equal(patchFromWorkspace, patchFromFS) {
			fmt.Printf("patches are equal, using the one provded via workspace\n")
			return patchFromWorkspace, nil
		} else {
			fmt.Printf("patches differ, using the one provded via workspace\n")
			return patchFromWorkspace, nil
		}
	case errWS != nil && errFS == nil:
		fmt.Printf("problem getting patch from workspace, fallback to filesystem: %s\n", errWS)
		return patchFromFS, nil
	case errWS == nil && errFS != nil:
		fmt.Printf("")
		fmt.Printf("problem getting patch from filesystem, fallback to workspace: %s\nPatch from filesystem:\n%s\n",
			errFS, patchFromFS)
		return patchFromWorkspace, nil
	case errWS != nil && errFS != nil:
		return nil, errors.Join(errWS, errFS)
	default:
		return nil, errors.New("unknown error getting patch")
	}
}

// getPatchFromFilesystem executes "git diff" using b/h as the references to compare
func getPatchFromFilesystem(b, h string) ([]byte, error) {
	cmd := exec.Command("git", "diff", b, h)
	patch, err := cmd.Output()
	if err != nil {
		return patch, fmt.Errorf("problem running %s: %w", cmd.String(), err)
	}

	if len(patch) == 0 {
		return nil, fmt.Errorf("patch empty")
	}

	return patch, nil
}

// getPatchFromWorkspace loads the data from file f and returns it or an error
func getPatchFromWorkspace(f string) ([]byte, error) {
	file, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	patch, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	if len(patch) == 0 {
		return nil, fmt.Errorf("patch empty")
	}

	return patch, nil
}

// postComment posts c as a comment on PR prNumber which is extracted from ref or returns an error
func postComment(c, repoOwner, repo, ref string) error {
	x := strings.Split(ref, "/")
	if len(x) < 3 {
		return fmt.Errorf("unable to extract PR number from ref %q", ref)
	}
	prNumber, err := strconv.Atoi(x[2])
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)))

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

	if githubToken == "" || openAiToken == "" {
		return fmt.Errorf("you need to set both GITHUB_TOKEN and OPENAI_TOKEN")
	}

	ref = os.Getenv("GITHUB_REF")
	base = os.Getenv("GITHUB_BASE_REF")
	head = os.Getenv("GITHUB_HEAD_REF")
	workspaceDir = os.Getenv("GITHUB_WORKSPACE")
	debug = os.Getenv("DEBUG") != ""

	if x := strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/"); len(x) == 2 {
		repoOwner = x[0]
		repoName = x[1]
	} else {
		return fmt.Errorf("GITHUB_REPOSITORY was in wrong format: %s", os.Getenv("GITHUB_REPOSITORY"))
	}

	return nil
}
