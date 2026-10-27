# Validly Go SDK

**Validly — Simple email validation for every stack.** Go client for the [Validly](../../README.md) API.

```sh
go get github.com/validly/validly/sdk/go
```

The SDK is its own module, versioned independently of the server, and has no
dependencies outside the standard library.

```go
import validly "github.com/validly/validly/sdk/go"

client := validly.NewClient(
	validly.WithBaseURL("http://localhost:8080"),
	validly.WithAPIKey(os.Getenv("VALIDLY_API_KEY")), // optional
	validly.WithTimeout(5*time.Second),
)

result, err := client.Validate(ctx, "person@example.com")
if err != nil {
	// Handle API, transport, or decoding errors.
}
fmt.Println(result.OverallStatus) // e.g. validly.OverallDomainDeliverable
```

- `Validate(ctx, email)` / `ValidateWithChecks(ctx, email, validly.Checks{SkipDNS: true})`
- `SendVerification(ctx, email)` and `ConfirmVerification(ctx, token)` (API key required)

## Errors

| Type                                            | When                                                                                                          |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `*validly.APIError`                             | The API answered with an error: `StatusCode`, `Code` (e.g. `validly.ErrRateLimited`), `Message`, `RetryAfter` |
| `*validly.TransportError`                       | The request did not complete; it may or may not have reached the server                                       |
| `*validly.DecodeError`                          | A success response could not be decoded                                                                       |
| `context.Canceled` / `context.DeadlineExceeded` | Your context ended                                                                                            |

`validly.IsCode(err, validly.ErrInvalidToken)` is a shortcut for API error codes.

## Timeouts and retries

`WithTimeout` (default 10 s) bounds each attempt; use a context deadline to
bound the whole call. Up to 2 retries (`WithRetries`) use exponential backoff
with jitter (`WithRetryBackoff`, 200 ms initially, capped at 5 s) and honour
`Retry-After`. They are made only for 429, 502, 503 (not
`verification_disabled`) and 504, and, for `Validate` only, for transport
errors. `SendVerification` and `ConfirmVerification` are never retried after
a transport error, because the first attempt may already have taken effect.

A `Client` is safe for concurrent use and holds no global state. Runnable
example: [`examples/go`](../../examples/go).
