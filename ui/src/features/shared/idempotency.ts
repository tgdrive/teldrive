import { newClientId } from "./client-id";

export function newIdempotencyKey() {
  return newClientId();
}
