import { MockArenaConsoleApi } from "./mockApi";
import { HttpArenaConsoleApi } from "./httpApi";
import type { ArenaConsoleApi } from "./types";

const configuredMode = import.meta.env.VITE_CONSOLE_DATA_MODE;
// A production build must never silently become a fixture console merely
// because the deploy command omitted an environment variable. Development
// still defaults to mock for convenience, and either mode can be explicit.
const mode = configuredMode === "http" || configuredMode === "mock"
  ? configuredMode
  : import.meta.env.PROD ? "http" : "mock";
const apiBasePath = import.meta.env.VITE_API_BASE_PATH ?? "/api";

export const arenaApi: ArenaConsoleApi =
  mode === "http" ? new HttpArenaConsoleApi({ basePath: apiBasePath }) : new MockArenaConsoleApi();

export const dataMode = mode;
