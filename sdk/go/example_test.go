package validly_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	validly "github.com/validly/validly/sdk/go"
)

func Example() {
	client := validly.NewClient(
		validly.WithBaseURL("http://localhost:8080"),
		validly.WithTimeout(5*time.Second),
	)

	result, err := client.Validate(context.Background(), "person@example.com")
	if err != nil {
		// Handle API, transport, or decoding errors.
		var apiErr *validly.APIError
		if errors.As(err, &apiErr) && apiErr.Code == validly.ErrRateLimited {
			fmt.Println("retry after", apiErr.RetryAfter)
		}
		return
	}

	switch result.OverallStatus {
	case validly.OverallInvalid:
		fmt.Println("Please enter a valid email address.")
	case validly.OverallUndeliverable:
		fmt.Println("This domain does not appear to accept email.")
	case validly.OverallUnknown:
		fmt.Println("We could not check the domain right now.")
	}
	if result.Suggestion != nil {
		fmt.Println("Did you mean", result.Suggestion.Email+"?")
	}
}
