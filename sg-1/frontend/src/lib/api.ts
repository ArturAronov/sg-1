import "server-only";
import { cache } from "react";
import { redirect } from "next/navigation";
import { requireSession } from "@/lib/auth";
import type { Me } from "@/lib/types";

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function backendFetch(
  path: string,
  token: string,
  init?: RequestInit,
): Promise<Response> {
  const base = process.env.SG1_API_URL;
  if (!base) throw new APIError(503, "SG1_API_URL is not configured");
  const headers = new Headers(init?.headers);
  headers.set("Authorization", `Bearer ${token}`);
  return fetch(new URL(path, base), {
    ...init,
    headers,
    cache: "no-store",
    signal: AbortSignal.timeout(8_000),
  });
}

export async function apiGet<T>(path: string): Promise<T> {
  const token = await requireSession();
  const response = await backendFetch(path, token);
  if (response.status === 401) redirect("/login?expired=1");
  if (!response.ok) throw new APIError(response.status, "The SG-1 API is unavailable");
  return response.json() as Promise<T>;
}

export const getMe = cache(() => apiGet<Me>("/v1/me"));
