// Always re-check on the server: client-side checks can be bypassed.
export default defineEventHandler(async (event) => {
  const { email } = await readBody<{ email?: unknown }>(event);
  if (typeof email !== "string") {
    throw createError({ statusCode: 400, message: "Please enter your email address." });
  }
  // useValidly() is auto-imported by @validly/nuxt and uses the private config.
  const check = await useValidly().check(email);
  if (!check.accepted) throw createError({ statusCode: 422, message: check.message });
  // Acceptance does not prove the mailbox exists: send a verification email next.
  return { registered: true };
});
