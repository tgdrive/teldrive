import createFetchClient from "openapi-fetch";
import createClient from "openapi-react-query";
import type { paths } from "@/lib/api";
import fetch from "./fetch-throw";

export const fetchClient = createFetchClient<paths>({
  baseUrl: "/api",
  requestInitExt: {
    signal: AbortSignal.timeout(180 * 1000),
  },
  headers: {
    "Content-Type": "application/json",
  },
  fetch,
});

export const $api = createClient(fetchClient);
