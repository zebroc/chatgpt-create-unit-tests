package main

import (
	"context"
	"fmt"
	"github.com/google/go-github/v51/github"
	"golang.org/x/oauth2"
	"io"
	"strconv"
	"strings"
)

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

func createAndSubmitReview(c, repoOwner, repo, ref string, comments []*github.DraftReviewComment) error {
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

	reviewReq := &github.PullRequestReviewRequest{
		Body:     &c,
		Event:    github.String("REQUEST_CHANGES"),
		Comments: comments,
	}

	review, resp, err := client.PullRequests.CreateReview(ctx, repoOwner, repo, prNumber, reviewReq)
	if err != nil {
		body, exx := io.ReadAll(resp.Body)
		if exx != nil {
			fmt.Printf("problem reading body: %s", err)
		}
		fmt.Printf("problem creating code review: %s\nBody: %s", err, body)
		return err
	}

	submittedReview, resp, err := client.PullRequests.SubmitReview(ctx, repoOwner, repo, prNumber, *review.ID, reviewReq)
	if err != nil {
		body, exx := io.ReadAll(resp.Body)
		if exx != nil {
			fmt.Printf("problem reading body: %s", err)
		}
		fmt.Printf("problem submitting code review: %s\nBody: %s", err, body)
		return err
	}

	fmt.Printf("%#v submitted", *submittedReview)

	return nil
}
