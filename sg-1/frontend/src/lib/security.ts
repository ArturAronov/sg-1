// Pure helpers shared by server-side security code and unit tests.
export function isSameOrigin(origin: string | null, publicURL: string): boolean {
  if (!origin) return false;
  try {
    return (
      new URL(origin).origin === new URL(publicURL).origin && origin === new URL(origin).origin
    );
  } catch {
    return false;
  }
}

export async function readLimitedBody(request: Request, limit = 16_384): Promise<string> {
  const reader = request.body?.getReader();
  if (!reader) return "";
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > limit) {
        await reader.cancel();
        throw new Error("Request is too large");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  return new TextDecoder("utf-8", { fatal: true }).decode(Buffer.concat(chunks));
}
