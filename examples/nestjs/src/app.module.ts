import { Module } from "@nestjs/common";
import { RegistrationController } from "./registration.controller.js";
import { validlyProvider } from "./validly.js";

@Module({ controllers: [RegistrationController], providers: [validlyProvider] })
export class AppModule {}
