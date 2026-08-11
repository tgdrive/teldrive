import { z } from "zod";

export const phoneNumberSchema = z
  .string()
  .regex(/^\+[1-9][0-9]{7,14}$/, "Use E.164 format, including the country code.");

export const telegramCodeSchema = z.string().trim().min(1, "Enter the code sent by Telegram.");

export const telegramPasswordSchema = z
  .string()
  .min(1, "Enter your Telegram two-step verification password.");

export type TelegramLoginValues = {
  phoneNumber: string;
  code: string;
  password: string;
};

export const defaultTelegramLoginValues: TelegramLoginValues = {
  phoneNumber: "",
  code: "",
  password: "",
};

export function firstFieldError(errors: unknown[]) {
  for (const error of errors) {
    if (typeof error === "string") {
      return error;
    }
    if (error && typeof error === "object" && "message" in error) {
      const message = (error as { message?: unknown }).message;
      if (typeof message === "string") {
        return message;
      }
    }
  }
  return undefined;
}
