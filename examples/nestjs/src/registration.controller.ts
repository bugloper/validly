import { Body, Controller, HttpCode, HttpException, Inject, Post } from "@nestjs/common";
import type { ServerValidly } from "@validly/server";
import { ValidEmailPipe } from "@validly/server/nestjs";
import { VALIDLY } from "./validly.js";

@Controller()
export class RegistrationController {
  private readonly validEmail: ValidEmailPipe;

  constructor(@Inject(VALIDLY) private readonly validly: ServerValidly) {
    this.validEmail = new ValidEmailPipe(validly);
  }

  /** Browsers call this instead of Validly, so the API key stays on the server. */
  @Post("api/validate")
  @HttpCode(200)
  async validate(@Body() body: unknown) {
    const r = await this.validly.handleValidateRequest(body);
    if (r.status !== 200) throw new HttpException(r.body as Record<string, unknown>, r.status);
    return r.body;
  }

  @Post("register")
  @HttpCode(201)
  async register(@Body("email") email: unknown) {
    const address = await this.validEmail.transform(email);
    // Acceptance does not prove the mailbox exists: send a verification email for that.
    return { registered: true, email: address };
  }
}
