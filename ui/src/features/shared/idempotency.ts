export function newIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  if (typeof crypto === "undefined" || typeof crypto.getRandomValues !== "function") {
    throw new Error("newIdempotencyKey requires crypto.randomUUID or crypto.getRandomValues");
  }
  return "10000000-1000-4000-8000-100000000000".replace(/[018]/g, (c) => {
    const value = Number(c);
    return (value ^ (crypto.getRandomValues(new Uint8Array(1))[0] & (15 >> (value / 4)))).toString(16);
  });
}
