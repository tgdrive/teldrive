let sequence = 0;

export function newClientId() {
  sequence = (sequence + 1) >>> 0;
  return `${Date.now().toString(36)}-${sequence.toString(36)}-${Math.random().toString(36).slice(2)}`;
}
