export function newIdempotencyKey() {
  return crypto.randomUUID();
}
