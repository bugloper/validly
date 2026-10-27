// Command example validates the addresses given on the command line with the
// Validly Go SDK.
//
//	VALIDLY_URL=http://localhost:8080 go run . person@example.com typo@gmial.com
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	validly "github.com/validly/validly/sdk/go"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: example <email>...")
		os.Exit(2)
	}
	baseURL := os.Getenv("VALIDLY_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	client := validly.NewClient(
		validly.WithBaseURL(baseURL),
		validly.WithAPIKey(os.Getenv("VALIDLY_API_KEY")), // optional
		validly.WithTimeout(5*time.Second),
	)

	exit := 0
	for _, email := range os.Args[1:] {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		result, err := client.Validate(ctx, email)
		cancel()
		if err != nil {
			var apiErr *validly.APIError
			if errors.As(err, &apiErr) {
				fmt.Printf("%s: API error %s (HTTP %d)\n", email, apiErr.Code, apiErr.StatusCode)
			} else {
				fmt.Printf("%s: %v\n", email, err)
			}
			exit = 1
			continue
		}
		fmt.Printf("%s: %s%s\n", email, result.OverallStatus, explain(result))
	}
	os.Exit(exit)
}

func explain(r *validly.Result) string {
	switch {
	case r.Syntax.Status == validly.SyntaxInvalid:
		return " (" + *r.Syntax.Reason + ")"
	case r.Suggestion != nil:
		return " (did you mean " + r.Suggestion.Email + "?)"
	case r.Disposable.Status == validly.Disposable:
		return " (disposable)"
	}
	// A deliverable domain does not prove the mailbox exists.
	return ""
}
