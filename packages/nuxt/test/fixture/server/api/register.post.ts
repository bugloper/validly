export default defineEventHandler(async (event) => {
  const { email } = await readBody<{ email?: string }>(event);
  const check = await useValidly().check(email ?? "");
  if (!check.accepted) {
    setResponseStatus(event, 422);
    return { error: check.reason };
  }
  return { ok: true };
});
