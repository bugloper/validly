"use server";

import { emailFieldError } from "@validly/nextjs/server";
import { validly } from "../lib/validly";

export interface SignupState {
  error: string | null;
  registered: string | null;
}

export async function signup(_prev: SignupState, formData: FormData): Promise<SignupState> {
  const email = formData.get("email");
  // Always re-check on the server: client-side checks can be bypassed.
  const error = await emailFieldError(validly, email);
  if (error) return { error, registered: null };
  // Acceptance does not prove the mailbox exists: send a verification email next.
  return { error: null, registered: String(email) };
}
