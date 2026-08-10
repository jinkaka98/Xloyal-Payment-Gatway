"use server";

import { timingSafeEqual } from "node:crypto";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { createSessionToken, SESSION_COOKIE, sessionCookieOptions } from "@/lib/session";

function equal(left: string, right: string) {
  const a = Buffer.from(left);
  const b = Buffer.from(right);
  return a.length === b.length && timingSafeEqual(a, b);
}

export async function login(formData: FormData) {
  const configuredEmail = process.env.ADMIN_CONSOLE_EMAIL ?? "admin@xloyal.id";
  const configuredPassword = process.env.ADMIN_CONSOLE_PASSWORD;
  const sessionSecret = process.env.CONSOLE_SESSION_SECRET;
  const email = String(formData.get("email") ?? "").trim().toLowerCase();
  const password = String(formData.get("password") ?? "");

  if (!configuredPassword || !sessionSecret || !equal(email, configuredEmail.toLowerCase()) || !equal(password, configuredPassword)) {
    redirect("/login?error=invalid");
  }

  (await cookies()).set(SESSION_COOKIE, await createSessionToken(sessionSecret), sessionCookieOptions);
  redirect("/dashboard");
}
