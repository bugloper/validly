# @validly/angular

**Validly — Simple email validation for every stack.** Angular integration for the [Validly](../../README.md) API: injectable service, Reactive Forms async validator, Observables and signals. Angular 20+.

```sh
pnpm add @validly/angular
```

```ts
// main.ts
bootstrapApplication(App, {
  providers: [provideValidly({ baseUrl: "https://validly.example.com" })],
});
```

## Reactive Forms

```ts
export class SignupComponent {
  private readonly validation = inject(EmailValidationService);

  readonly email = new FormControl("", {
    nonNullable: true,
    validators: [Validators.required],
    asyncValidators: [this.validation.validator({ debounceMs: 400 })],
  });
}
```

```html
<input type="email" autocomplete="email" [formControl]="email" />
@if (email.pending) { Checking email... }
@if (email.errors?.["validly"]; as error) { {{ error.message }} }
```

When an address is rejected, the control gets a `validly` error:
`{ reason, message, result }`, where `reason` is `invalid_syntax`,
`undeliverable_domain`, `disposable`, `unknown_domain`, `role` or
`unavailable`.

Validator options:

- `debounceMs` (400). Angular unsubscribes from the previous check when the
  value changes, which cancels the timer and aborts the HTTP request.
- `policy`: what to reject (see `evaluate` in `@validly/core`).
- `whenUnavailable`: `"allow"` (default) treats the value as valid when
  Validly cannot be reached, so an outage does not block your form;
  `"reject"` sets `{ validly: { reason: "unavailable" } }`.
- `onResult(result | null)`: receives every result, e.g. to show
  "Did you mean …?" for accepted addresses.

Malformed input is rejected synchronously by the local syntax check, without
a request.

## Observables and signals

```ts
this.validation.validate(email).subscribe((result) => ...); // unsubscribe aborts the request

readonly v = injectEmailValidation({ debounceMs: 400 }); // in an injection context
// v.status(), v.result(), v.loading(), v.message(), v.validate(value); cleaned up with the component
```

The package uses no decorators, so it needs no Angular compiler step and works
in zoneless applications. Example: [`examples/angular`](../../examples/angular).
