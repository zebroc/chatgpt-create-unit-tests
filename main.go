package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/go-github/v51/github"
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
	maxPatchSize                           = 10000
	prompts                                = map[string]string{
		"Unit tests": "If there are any new functions in this patch that do not already have a unit test, " +
			"write a unit test for each of them\n\n%s",
		"Code review":        "Please perform a code review for this patch:\n\n%s",
		"Scalability review": "Review the given patch for potential scalability issues:\n\n%s",
		"Security review":    "Review the given patch for potential security issues:\n\n%s",
	}
	reviewPrompts = map[string]string{
		"Code review": "Given the following patch:\\n\\n%s\\n\\nplease perform a code review and create GitHub Review comments suggesting code changes and fill each one into a JSON object like: { \"path\": \"\", \"body\": \"FILL IN SUGGESTION\\n\\```suggestion\\nCODE```\", \"start_side\": \"RIGHT\", \"side\": \"RIGHT\", \"start_line\":  STARTING_LINE, \"line\": ENDING_LINE } and then return just those objects in an array.",
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

	if len(patch) > maxPatchSize {
		_ = postComment(fmt.Sprintf("Size of patch (%d) too big, unable to prompt OpenAI. Consider splitting the PR\n", len(patch)), repoOwner, repoName, ref)
		Exit(fmt.Sprintf("Size of patch (%d) too big, unable to prompt OpenAI. Consider splitting the PR\n", len(patch)), 2)
	}

	var wg sync.WaitGroup
	wg.Add(len(prompts))
	wg.Add(len(reviewPrompts))
	for name, prompt := range prompts {
		PromptAndComment(patch, name, prompt, &wg)
	}

	for name, prompt := range reviewPrompts {
		PromptAndReview(patch, name, prompt, &wg)
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

	err = postComment("##"+name+"\n"+response.Choices[0].Message.Content,
		repoOwner, repoName, ref)
	if err != nil {
		fmt.Printf("unable to post comment: %v\n", err)
	}
}

// PromptAndReview executes prompt with patch and creates a code review on the PR or logs an error
func PromptAndReview(patch []byte, name, prompt string, wg *sync.WaitGroup) {
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

	var x []github.DraftReviewComment
	err = json.Unmarshal([]byte(response.Choices[0].Message.Content), &x)
	if err != nil {
		fmt.Printf("problem extracting codereview JSON object: %s\nResponse: %s",
			err, response.Choices[0].Message.Content)
	}

	var xp []*github.DraftReviewComment
	for _, c := range x {
		xp = append(xp, &c)
	}
	err = createAndSubmitReview("Review", repoOwner, repoName, ref, xp)
	if err != nil {
		fmt.Printf("problem submitting code review: %s", err)
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
		DebugPrint("problem getting patch from workspace, fallback to filesystem: %s\n", errWS)
		return patchFromFS, nil
	case errWS == nil && errFS != nil:
		DebugPrint("problem getting patch from filesystem, fallback to workspace: %s\n", errFS)
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

	// Debug?
	if os.Getenv("DEBUG") != "" || os.Getenv("INPUT_DEBUG") != "false" {
		debug = true
	}

	if size, ok := os.LookupEnv("INPUT_MAXPATCHSIZE"); ok {
		if i, err := strconv.Atoi(size); err == nil && i > 0 {
			maxPatchSize = i
		}
	}

	// See if there are custom prompts
	if x, ok := os.LookupEnv("INPUT_PROMPTS"); ok {
		var p map[string]string
		err := json.Unmarshal([]byte(x), &p)
		if err == nil && len(p) > 0 {
			prompts = p
		}
	}

	if x := strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/"); len(x) == 2 {
		repoOwner = x[0]
		repoName = x[1]
	} else {
		return fmt.Errorf("GITHUB_REPOSITORY was in wrong format: %s", os.Getenv("GITHUB_REPOSITORY"))
	}

	return nil
}
