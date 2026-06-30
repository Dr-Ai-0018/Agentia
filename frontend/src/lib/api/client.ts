import { MockArenaConsoleApi } from "./mockApi";
import { HttpArenaConsoleApi } from "./httpApi";
import type { ArenaConsoleApi } from "./types";

const mode = import.meta.env.VITE_CONSOLE_DATA_MODE ?? "mock";
const apiBasePath = import.meta.env.VITE_API_BASE_PATH ?? "/api";

export const arenaApi: ArenaConsoleApi =
  mode === "http" ? new HttpArenaConsoleApi({ basePath: apiBasePath }) : new MockArenaConsoleApi();

export const dataMode = mode;
