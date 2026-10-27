// Angular's npm packages are partially compiled and need the JIT compiler
// when run outside the Angular CLI.
import "@angular/compiler";
import { TestBed } from "@angular/core/testing";
import { BrowserTestingModule, platformBrowserTesting } from "@angular/platform-browser/testing";

TestBed.initTestEnvironment(BrowserTestingModule, platformBrowserTesting());
