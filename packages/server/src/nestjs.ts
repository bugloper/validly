import {
  BadRequestException,
  type PipeTransform,
  ServiceUnavailableException,
  UnprocessableEntityException,
} from "@nestjs/common";
import { rejectionBody, type ServerValidly } from "./index.js";

/**
 * A NestJS pipe that validates an email parameter and applies the policy:
 *
 *   @Post("register")
 *   register(@Body("email", new ValidEmailPipe(validly)) email: string) { ... }
 *
 * Throws 400 for a missing value, 422 for a rejected address and 503 when
 * Validly is unreachable and `whenUnavailable` is "reject".
 */
export class ValidEmailPipe implements PipeTransform<unknown, Promise<string>> {
  constructor(private readonly validly: ServerValidly) {}

  async transform(value: unknown): Promise<string> {
    if (typeof value !== "string") {
      throw new BadRequestException({ error: { code: "invalid_request", message: "An email address is required." } });
    }
    const check = await this.validly.check(value);
    if (!check.accepted) {
      const body = rejectionBody(check);
      throw check.reason === "unavailable"
        ? new ServiceUnavailableException(body)
        : new UnprocessableEntityException(body);
    }
    return value;
  }
}
