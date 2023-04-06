package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	GPT3Dot5Turbo = "gpt-3.5-turbo"
	openAiBaseURL = "https://api.openai.com/v1"
)

// Prompt takes in a string prompt p and returns a ChatGPTResponse or an error
func Prompt(p string) (ChatGPTResponse, error) {
	req := ChatGPTRequest{
		Model: GPT3Dot5Turbo,
		Messages: []Message{
			{
				Role:    "user",
				Content: p,
			},
		},
	}

	response, err := executeRequest(openAiBaseURL, openAiToken, &req)
	usage += response.Usage.TotalTokens
	return response, err
}

func executeRequest(u, t string, r *ChatGPTRequest) (ChatGPTResponse, error) {
	var response ChatGPTResponse

	client := http.Client{}
	req, err := http.NewRequest("POST", u+"/chat/completions",
		bytes.NewBuffer(r.JSON()))
	if err != nil {
		return response, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+t)
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return response, fmt.Errorf("error doing request: %v", err)
	}

	var data []byte
	if data, err = io.ReadAll(resp.Body); err != nil {
		return response, fmt.Errorf("error receicing data: %v", err)
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return ChatGPTResponse{}, fmt.Errorf("error unmarshalling data: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 300 {
		return response, fmt.Errorf("error doing request: %d\n%s", resp.StatusCode, data)
	}

	if resp.Body != nil {
		_ = resp.Body.Close()
	}

	return response, nil
}

type ChatGPTRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatGPTResponse struct {
	Id      string   `json:"id"`
	Object  string   `json:"object"`
	Created int      `json:"created"`
	Model   string   `json:"model"`
	Usage   Usage    `json:"usage"`
	Choices []Choice `json:"choices"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
	Index        int     `json:"index"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (r ChatGPTRequest) JSON() []byte {
	d, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	return d
}
