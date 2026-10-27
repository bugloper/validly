import "reflect-metadata";
import { NestFactory } from "@nestjs/core";
import { AppModule } from "./app.module.js";

async function bootstrap() {
  const app = await NestFactory.create(AppModule, { bodyParser: true });
  const port = Number(process.env.PORT ?? 3004);
  await app.listen(port, "127.0.0.1");
  console.log(`nestjs example on http://localhost:${port}`);
}

void bootstrap();
