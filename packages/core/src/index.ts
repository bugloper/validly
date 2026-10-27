export {
  type ClientOptions,
  createClient,
  type EmailValidator,
  type RequestOptions,
  type ValidateOptions,
  type ValidlyClient,
} from "./client.js";
export {
  type ControllerOptions,
  createValidationController,
  IDLE_STATE,
  type ValidationController,
  type ValidationState,
  type ValidationStatus,
} from "./controller.js";
export { describeResult, type ResultMessage } from "./describe.js";
export { isValidlyError, ValidlyError, type ValidlyErrorKind } from "./errors.js";
export { evaluate, type Policy, type PolicyDecision, type RejectionReason } from "./policy.js";
export { checkSyntax, type LocalSyntaxResult, localInvalidResult } from "./syntax.js";
export * from "./types.js";
