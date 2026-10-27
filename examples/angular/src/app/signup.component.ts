import { Component, inject, signal } from "@angular/core";
import { toSignal } from "@angular/core/rxjs-interop";
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from "@angular/forms";
import { EmailValidationService, type ValidationResult, type ValidlyValidationError } from "@validly/angular";

@Component({
  selector: "app-root",
  imports: [ReactiveFormsModule],
  template: `
    <main>
      <h1>Validly — Angular</h1>
      <form [formGroup]="form" (ngSubmit)="submit()" novalidate>
        <label for="email">Email</label>
        <input
          id="email"
          type="email"
          autocomplete="email"
          formControlName="email"
          aria-describedby="email-status"
          [attr.aria-invalid]="status() === 'INVALID'"
          [attr.aria-busy]="status() === 'PENDING'"
        />
        <p id="email-status" role="status" aria-live="polite">
          @if (status() === "PENDING") {
            Checking email...
          } @else if (validlyError(); as error) {
            {{ error.message }}
          } @else if (unavailable()) {
            We could not check this address right now, but you can continue.
          }
          @if (suggestion(); as s) {
            Did you mean {{ s }}?
            <button type="button" (click)="useSuggestion(s)">Use it</button>
          }
        </p>
        <button type="submit">Create account</button>
        @if (submitted(); as address) {
          <p>Thanks! We sent a confirmation link to {{ address }}.</p>
        }
      </form>
    </main>
  `,
})
export class SignupComponent {
  private readonly validation = inject(EmailValidationService);

  readonly suggestion = signal<string | null>(null);
  readonly unavailable = signal(false);
  readonly submitted = signal<string | null>(null);

  readonly email = new FormControl("", {
    nonNullable: true,
    validators: [Validators.required],
    // Debounced; Angular cancels the previous check when the value changes.
    // If Validly is unreachable the address is allowed (whenUnavailable: "allow").
    asyncValidators: [
      this.validation.validator({
        debounceMs: 400,
        onResult: (result: ValidationResult | null) => {
          this.unavailable.set(result === null);
          this.suggestion.set(result?.suggestion?.email ?? null);
        },
      }),
    ],
  });
  readonly form = new FormGroup({ email: this.email });

  // Signals drive zoneless change detection.
  readonly status = toSignal(this.email.statusChanges, { initialValue: this.email.status });

  validlyError(): ValidlyValidationError | null {
    return (this.email.errors?.["validly"] as ValidlyValidationError | undefined) ?? null;
  }

  useSuggestion(address: string): void {
    // Only change the address when the user asks to.
    this.email.setValue(address);
  }

  submit(): void {
    if (this.form.pending || this.form.invalid) {
      this.email.markAsTouched();
      return;
    }
    this.submitted.set(this.email.value);
  }
}
