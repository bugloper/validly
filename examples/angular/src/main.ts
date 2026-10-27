import { provideZonelessChangeDetection } from "@angular/core";
import { bootstrapApplication } from "@angular/platform-browser";
import { provideValidly } from "@validly/angular";
import { SignupComponent } from "./app/signup.component";

bootstrapApplication(SignupComponent, {
  providers: [
    provideZonelessChangeDetection(),
    // Browser requests are anonymous: never put an API key in frontend code.
    provideValidly({ baseUrl: "http://localhost:8080" }),
  ],
}).catch((err: unknown) => console.error(err));
